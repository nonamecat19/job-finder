package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultRetentionDays = 30

const maxPagesPerRun = 50

const pageSize = 100

type Config struct {
	BaseURL string

	PublicKey string
	SecretKey string

	RetentionDays int
}

func (c Config) enabled() bool {
	return c.BaseURL != "" && c.PublicKey != "" && c.SecretKey != ""
}

func (c Config) window() time.Duration {
	days := c.RetentionDays
	if days <= 0 {
		days = DefaultRetentionDays
	}
	return time.Duration(days) * 24 * time.Hour
}

type Report struct {
	Skipped bool

	Cutoff time.Time

	Deleted int

	Truncated bool
}

type Pruner struct {
	cfg    Config
	client *http.Client
	now    func() time.Time
}

func New(cfg Config, client *http.Client) *Pruner {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Pruner{cfg: cfg, client: client, now: time.Now}
}

func (p *Pruner) Prune(ctx context.Context) (Report, error) {
	if !p.cfg.enabled() {
		return Report{Skipped: true}, nil
	}

	cutoff := p.now().UTC().Add(-p.cfg.window())
	report := Report{Cutoff: cutoff}

	for page := 1; page <= maxPagesPerRun; page++ {
		ids, err := p.listBefore(ctx, cutoff)
		if err != nil {
			return report, fmt.Errorf("list records before %s: %w", cutoff.Format(time.RFC3339), err)
		}
		if len(ids) == 0 {
			return report, nil
		}
		if err := p.deleteBatch(ctx, ids); err != nil {
			return report, fmt.Errorf("delete %d records: %w", len(ids), err)
		}
		report.Deleted += len(ids)

		if len(ids) < pageSize {
			return report, nil
		}
	}

	report.Truncated = true
	return report, nil
}

type traceListResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (p *Pruner) listBefore(ctx context.Context, cutoff time.Time) ([]string, error) {
	url := fmt.Sprintf("%s/api/public/traces?toTimestamp=%s&limit=%d&page=1",
		strings.TrimRight(p.cfg.BaseURL, "/"), cutoff.Format(time.RFC3339), pageSize)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(p.cfg.PublicKey, p.cfg.SecretKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(resp)
	}

	var parsed traceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode listing: %w", err)
	}
	ids := make([]string, 0, len(parsed.Data))
	for _, t := range parsed.Data {
		if t.ID != "" {
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}

func (p *Pruner) deleteBatch(ctx context.Context, ids []string) error {
	body, err := json.Marshal(map[string]any{"traceIds": ids})
	if err != nil {
		return err
	}
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/api/public/traces"

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(p.cfg.PublicKey, p.cfg.SecretKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return statusError(resp)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func statusError(resp *http.Response) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("collector returned %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
}
