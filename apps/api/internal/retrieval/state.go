package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nonamecat19/job-scraper/ports"

	"github.com/job-finder/api/internal/crypto"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

// StateStore is the app-side implementation of the library's StateStorePort:
// it owns the Postgres row shape and the cookie encryption, so the engine only
// ever sees plaintext library types.
var _ ports.StateStore = (*StateStore)(nil)

type StateStore struct {
	q             *sqlcgen.Queries
	encryptionKey string
}

func NewStateStore(q *sqlcgen.Queries, encryptionKey string) *StateStore {
	return &StateStore{q: q, encryptionKey: encryptionKey}
}

func (s *StateStore) Get(ctx context.Context, host string) (*ports.HostState, error) {
	row, err := s.q.GetHostRetrievalState(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("retrieval: get state for %s: %w", host, err)
	}
	if s.encryptionKey != "" && len(row.Cookies) > 0 {
		var cookies map[string]string
		if err := crypto.DecryptJSON(s.encryptionKey, string(row.Cookies), &cookies); err == nil {
			data, _ := json.Marshal(cookies)
			row.Cookies = data
		}
	}
	return &ports.HostState{
		Host:               row.Host,
		IdentityVersion:    row.IdentityVersion,
		CurrentRung:        row.CurrentRung,
		RungLastVerifiedAt: timestamptzPtr(row.RungLastVerifiedAt),
		Cookies:            row.Cookies,
		ConsecutiveBlocks:  row.ConsecutiveBlocks,
		CoolingOffUntil:    timestamptzPtr(row.CoolingOffUntil),
		LastBlockAt:        timestamptzPtr(row.LastBlockAt),
		LastBlockReason:    row.LastBlockReason,
		CrawlDelaySeconds:  row.CrawlDelaySeconds,
	}, nil
}

func timestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func timestamptzFrom(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func (s *StateStore) Upsert(ctx context.Context, host string, state *ports.HostState) error {
	var cookies []byte
	if state.Cookies != nil {
		if s.encryptionKey != "" {
			var cookiesMap map[string]string
			if err := json.Unmarshal(state.Cookies, &cookiesMap); err == nil {
				encrypted, err := crypto.EncryptJSON(s.encryptionKey, cookiesMap)
				if err != nil {
					return fmt.Errorf("retrieval: encrypt cookies: %w", err)
				}
				cookies = []byte(encrypted)
			}
		}
	}
	if cookies == nil {
		cookies = state.Cookies
	}

	params := sqlcgen.UpsertHostRetrievalStateParams{
		Host:               host,
		IdentityVersion:    state.IdentityVersion,
		CurrentRung:        state.CurrentRung,
		RungLastVerifiedAt: timestamptzFrom(state.RungLastVerifiedAt),
		Cookies:            cookies,
		ConsecutiveBlocks:  state.ConsecutiveBlocks,
		CoolingOffUntil:    timestamptzFrom(state.CoolingOffUntil),
		LastBlockAt:        timestamptzFrom(state.LastBlockAt),
		LastBlockReason:    state.LastBlockReason,
		CrawlDelaySeconds:  state.CrawlDelaySeconds,
	}
	return s.q.UpsertHostRetrievalState(ctx, params)
}

func (s *StateStore) ClearCookies(ctx context.Context, host string) error {
	return s.q.ClearHostCookies(ctx, host)
}

func (s *StateStore) ClearRung(ctx context.Context, host string) error {
	return s.q.ClearHostRung(ctx, host)
}

func (s *StateStore) RecordBlock(ctx context.Context, host string, reason string) error {
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	return s.q.RecordHostBlock(ctx, sqlcgen.RecordHostBlockParams{
		Host:            host,
		LastBlockReason: reasonPtr,
	})
}

func (s *StateStore) RecordSuccess(ctx context.Context, host string, rung string) error {
	return s.q.RecordHostSuccess(ctx, sqlcgen.RecordHostSuccessParams{
		Host:        host,
		CurrentRung: rung,
	})
}

var crawlDelayRe = regexp.MustCompile(`(?im)^[Cc]rawl-[Dd]elay:\s*(\d+)`)

func (s *StateStore) FetchAndSetCrawlDelay(ctx context.Context, host string) error {
	state, err := s.Get(ctx, host)
	if err != nil {
		return fmt.Errorf("retrieval: get state for crawl delay: %w", err)
	}
	if state.CrawlDelaySeconds != nil {
		return nil
	}

	robotsURL := "https://" + host + "/robots.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return fmt.Errorf("retrieval: build robots.txt request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("retrieval: fetch robots.txt from %s: %w", host, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("retrieval: read robots.txt: %w", err)
	}

	delay := parseCrawlDelay(string(body))
	delayPtr := &delay
	return s.q.SetHostCrawlDelay(ctx, sqlcgen.SetHostCrawlDelayParams{
		Host:              host,
		CrawlDelaySeconds: delayPtr,
	})
}

func parseCrawlDelay(body string) int32 {
	matches := crawlDelayRe.FindStringSubmatch(body)
	if len(matches) >= 2 {
		val, err := strconv.Atoi(strings.TrimSpace(matches[1]))
		if err == nil && val > 0 {
			return int32(val)
		}
	}
	return 0
}

func (s *StateStore) LoadCookies(ctx context.Context, host string) ([]*http.Cookie, error) {
	row, err := s.Get(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(row.Cookies) == 0 {
		return nil, nil
	}
	var raw map[string]string
	if err := json.Unmarshal(row.Cookies, &raw); err != nil {
		return nil, fmt.Errorf("retrieval: decode cookies: %w", err)
	}
	return deserializeCookies(raw), nil
}

func (s *StateStore) SaveCookies(ctx context.Context, host string, cookies []*http.Cookie) error {
	raw := serializeCookies(cookies)
	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("retrieval: marshal cookies: %w", err)
	}
	if s.encryptionKey != "" {
		encrypted, err := crypto.EncryptJSON(s.encryptionKey, raw)
		if err != nil {
			return fmt.Errorf("retrieval: encrypt cookies: %w", err)
		}
		return s.q.UpdateHostCookies(ctx, sqlcgen.UpdateHostCookiesParams{
			Host:    host,
			Cookies: []byte(encrypted),
		})
	}
	return s.q.UpdateHostCookies(ctx, sqlcgen.UpdateHostCookiesParams{
		Host:    host,
		Cookies: data,
	})
}

func serializeCookies(cookies []*http.Cookie) map[string]string {
	out := make(map[string]string, len(cookies))
	for _, c := range cookies {
		out[c.Name] = c.Value
	}
	return out
}

func deserializeCookies(raw map[string]string) []*http.Cookie {
	cookies := make([]*http.Cookie, 0, len(raw))
	for name, value := range raw {
		cookies = append(cookies, &http.Cookie{Name: name, Value: value})
	}
	return cookies
}

func TimestamptzPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.UTC().Format("2006-01-02T15:04:05Z")
	return &s
}
