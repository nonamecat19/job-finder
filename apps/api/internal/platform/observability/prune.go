// Package observability owns the platform's half of the LLM observability
// arrangement introduced by feature 036: the retention guarantee.
//
// Collection itself needs no Go code — the routing service's success and
// failure callbacks record every routed call, which is what makes 036's US1
// config-only. What the routing service does *not* do is forget. Automated
// retention is an enterprise feature of the collector (research R7), so an
// OSS self-host that promises a 30-day window has to enforce it itself.
//
// That is what this package is. It is deliberately small and deliberately
// separate from the LLM path: nothing here runs on a request, and a failure
// here must never affect inference.
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

// DefaultRetentionDays is the window the platform promises when nothing is
// configured (036 FR-008).
const DefaultRetentionDays = 30

// maxPagesPerRun bounds one pruning pass. A run that hits the bound deletes
// what it found and says so; the next tick continues. This exists so a
// long-neglected collector cannot turn one scheduled tick into an unbounded
// loop against its API.
const maxPagesPerRun = 50

// pageSize is the collector's list page size. Its API caps this well below
// what a batch delete accepts, so the two are separate numbers.
const pageSize = 100

// Config is what the pruner needs to reach the collector.
//
// The credentials here are the collector's own API keys, not provider
// credentials — the application still holds no CEREBRAS/GROQ/COHERE/OPENROUTER
// key, which is the clause Constitution Principle V actually protects. They are
// named EVAL_PRUNE_* rather than LANGFUSE_* on purpose: 036 C2-2 forbids
// granting the application container a LANGFUSE_* variable, and the pruning job
// is a different grant with a different reason, so it reads different names and
// can be revoked on its own.
type Config struct {
	// BaseURL is the collector's API root. Empty disables pruning entirely.
	BaseURL string
	// PublicKey and SecretKey authenticate against the collector's API. Either
	// one empty disables pruning entirely.
	PublicKey string
	SecretKey string
	// RetentionDays is the window. Zero or negative means DefaultRetentionDays.
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

// Report is what one pruning run did. It exists because FR-008a asks for the
// guarantee to be observable: "records older than 30 days are deleted" is a
// claim, and a claim nobody can check is how a retention window quietly stops
// being enforced.
type Report struct {
	// Skipped is true when the collector is not configured. Not an error: a
	// deployment that never turned collection on has nothing to prune.
	Skipped bool
	// Cutoff is the boundary this run used. Anything recorded before it was
	// eligible for deletion.
	Cutoff time.Time
	// Deleted counts records this run removed.
	Deleted int
	// Truncated is true when the run hit its page bound with more to do, so a
	// zero-deleted next run is not mistaken for "nothing left".
	Truncated bool
}

// Pruner deletes collector records older than the retention window.
type Pruner struct {
	cfg    Config
	client *http.Client
	now    func() time.Time
}

// New builds a pruner. A nil client gets a bounded default: the collector is a
// peer service on the same host, and a pruning run that hangs is a scheduled
// tick that never finishes.
func New(cfg Config, client *http.Client) *Pruner {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Pruner{cfg: cfg, client: client, now: time.Now}
}

// Prune deletes every record older than the window and reports what it removed.
//
// Errors are returned, never swallowed (FR-008a). A retention job that fails
// silently is worse than none: the window still appears in the documentation
// while the data accumulates.
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
		// Always re-list from the first page rather than paginating forward:
		// the rows just deleted are gone, so page 1 is the next batch. Walking
		// page numbers over a shrinking result set skips records.
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

// statusError turns a non-success response into an error carrying enough of the
// body to diagnose it — an auth failure and a missing endpoint are both 4xx and
// need very different fixes.
func statusError(resp *http.Response) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("collector returned %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
}
