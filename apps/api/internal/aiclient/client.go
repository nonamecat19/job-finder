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

const DefaultTimeout = 30 * time.Second

type RequestContext struct {
	UserID     string  `json:"user_id"`
	WorkID     string  `json:"work_id"`
	ActivityID *string `json:"activity_id,omitempty"`
}

type wireRequest struct {
	Input   json.RawMessage `json:"input"`
	Context RequestContext  `json:"context"`
}

type Response struct {
	Status  events.ResultStatus `json:"status"`
	Result  json.RawMessage     `json:"result,omitempty"`
	Failure *events.Failure     `json:"failure,omitempty"`
	TraceID string              `json:"trace_id,omitempty"`
	Usage   *events.Usage       `json:"usage,omitempty"`
}

type Client struct {
	baseURL        string
	token          string
	httpc          *http.Client
	timeouts       map[string]time.Duration
	defaultTimeout time.Duration
	logger         *slog.Logger
}

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

func (c *Client) timeoutFor(capability string) time.Duration {
	if t, ok := c.timeouts[capability]; ok {
		return t
	}
	return c.defaultTimeout
}

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
	httpReq.Header.Set("X-Request-Id", requestID)

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

func retryAfterDelay(header string) time.Duration {
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}
