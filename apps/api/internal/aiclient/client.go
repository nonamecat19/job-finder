// Package aiclient is the Go-side client for the AI service's interactive
// HTTP surface (contracts/http.md H1-H8): capabilities invoked while a user
// waits (rephrase, recruiter, outreach, embed). Queued capabilities travel
// as events (internal/events), not through this package.
package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/job-finder/api/internal/events"
)

// DefaultTimeout is used for a capability absent from Client's timeouts map.
const DefaultTimeout = 30 * time.Second

// RequestContext is H3's context object — what makes the run's trace
// findable (H3-2, FR-014) and lets the backend correlate an activity.
type RequestContext struct {
	UserID     string  `json:"user_id"`
	WorkID     string  `json:"work_id"`
	ActivityID *string `json:"activity_id,omitempty"`
}

type wireRequest struct {
	Input   json.RawMessage `json:"input"`
	Context RequestContext  `json:"context"`
}

// Response mirrors the result event shape (H4-1, contracts/events.md E4),
// so both transports produce one result type.
type Response struct {
	Status  events.ResultStatus `json:"status"`
	Result  json.RawMessage     `json:"result,omitempty"`
	Failure *events.Failure     `json:"failure,omitempty"`
	TraceID string              `json:"trace_id,omitempty"`
	Usage   *events.Usage       `json:"usage,omitempty"`
}

// Client calls capabilities on the AI service's interactive HTTP surface,
// authenticating with the shared secret H7-1 requires.
type Client struct {
	baseURL        string
	token          string
	httpc          *http.Client
	timeouts       map[string]time.Duration
	defaultTimeout time.Duration
	logger         *slog.Logger
}

// New builds a Client. timeouts gives a per-capability whole-request
// timeout — H6-1 requires it exceed the capability's whole-run timeout
// (C4-4); a capability absent from timeouts uses defaultTimeout
// (DefaultTimeout if defaultTimeout <= 0).
func New(baseURL, token string, timeouts map[string]time.Duration, defaultTimeout time.Duration, logger *slog.Logger) *Client {
	if defaultTimeout <= 0 {
		defaultTimeout = DefaultTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		token:          token,
		httpc:          &http.Client{},
		timeouts:       timeouts,
		defaultTimeout: defaultTimeout,
		logger:         logger,
	}
}

// timeoutFor returns capability's configured whole-request timeout, or the
// client's default when none is configured for it.
func (c *Client) timeoutFor(capability string) time.Duration {
	if t, ok := c.timeouts[capability]; ok {
		return t
	}
	return c.defaultTimeout
}

// Invoke calls capability's HTTP surface (H2) with input (validated against
// the capability's declared input model server-side, H3-1 — a mismatch
// comes back as a 422 Response.Failure, not a Go error) and reqCtx (H3-2,
// H3-3).
//
// A non-nil error means the call never produced a classified result at all
// — the service was unreachable, the request could not be built, or the
// response body did not match the expected shape (H6-4: no fallback, no
// substituted result). A classified capability failure — invalid input,
// rate limiting, an unavailable provider, a bound exceeded — comes back as
// a non-nil Response with Response.Failure set and a nil error; the caller
// decides what that means for its own flow (H4-2's status code is present
// on Response but callers should switch on Failure.Category, which is what
// survives the transport).
//
// trace_id is logged on every response, success or failure (H6-3), so a
// user-facing error is traceable without searching Langfuse by hand.
//
// The client never retries a non-retryable failure (H6-2). A rate_limited
// response is retried exactly once, honouring Retry-After when the header
// is present; any other retryable category is left to the caller, which
// already owns retry/backoff policy for its own synchronous request path.
func (c *Client) Invoke(ctx context.Context, capability string, input any, reqCtx RequestContext) (*Response, error) {
	resp, retryAfter, err := c.invokeOnce(ctx, capability, input, reqCtx)
	if err != nil {
		return nil, err
	}

	if resp.Status == events.ResultFailed && resp.Failure != nil &&
		resp.Failure.Category == events.FailureRateLimited && resp.Failure.Retryable {
		delay := retryAfter
		if delay <= 0 {
			delay = 0
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return resp, nil
		}
		retried, _, err := c.invokeOnce(ctx, capability, input, reqCtx)
		if err != nil {
			// The retry attempt failed at the transport level; the
			// original classified rate_limited response is still the more
			// useful thing to hand back than a transport error (H6-4).
			return resp, nil
		}
		return retried, nil
	}

	return resp, nil
}

func (c *Client) invokeOnce(ctx context.Context, capability string, input any, reqCtx RequestContext) (*Response, time.Duration, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, 0, fmt.Errorf("aiclient: marshal input: %w", err)
	}
	body, err := json.Marshal(wireRequest{Input: inputJSON, Context: reqCtx})
	if err != nil {
		return nil, 0, fmt.Errorf("aiclient: marshal request: %w", err)
	}

	timeout := c.timeoutFor(capability)
	reqCtx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := c.baseURL + "/v1/capabilities/" + capability + "/invoke"
	httpReq, err := http.NewRequestWithContext(reqCtx2, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("aiclient: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	requestID := uuid.NewString()
	httpReq.Header.Set("X-Request-Id", requestID) // H3-4: client-generated request id

	httpResp, err := c.httpc.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("aiclient: invoke %s: %w", capability, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusNotFound {
		return nil, 0, fmt.Errorf("aiclient: invoke %s: unknown capability (404)", capability)
	}

	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("aiclient: invoke %s: read response: %w", capability, err)
	}

	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, 0, fmt.Errorf("aiclient: invoke %s: response status %d did not decode as a result: %w", capability, httpResp.StatusCode, err)
	}

	c.logResult(capability, requestID, httpResp.StatusCode, resp)

	return &resp, retryAfterDelay(httpResp.Header.Get("Retry-After")), nil
}

func (c *Client) logResult(capability, requestID string, status int, resp Response) {
	fields := []any{
		"capability", capability,
		"request_id", requestID,
		"http_status", status,
		"status", resp.Status,
		"trace_id", resp.TraceID,
	}
	if resp.Status == events.ResultFailed && resp.Failure != nil {
		fields = append(fields, "failure_category", resp.Failure.Category, "retryable", resp.Failure.Retryable)
		c.logger.Error("aiclient: capability invocation failed", fields...)
		return
	}
	c.logger.Info("aiclient: capability invoked", fields...)
}

// retryAfterDelay parses a Retry-After header value as a number of seconds
// (the AI service's expected form for a rate_limited response). An absent
// or unparseable header yields zero, so callers wait no longer than
// necessary rather than guessing.
func retryAfterDelay(header string) time.Duration {
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}
