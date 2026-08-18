//go:build integration

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/job-finder/api/internal/testinfra"
)

type stub struct {
	mu       sync.Mutex
	requests []stubRequest
	failing  map[string]bool
}

type stubRequest struct {
	path       string
	model      string
	dimensions int
}

var upstream = &stub{failing: map[string]bool{}}

func (s *stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Model      string `json:"model"`
		Dimensions int    `json:"dimensions"`
	}
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &payload)

	s.mu.Lock()
	s.requests = append(s.requests, stubRequest{path: r.URL.Path, model: payload.Model, dimensions: payload.Dimensions})
	failing := s.failing[payload.Model]
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if failing {

		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"stub upstream is failing this model","type":"server_error"}}`)
		return
	}

	switch {
	case strings.HasSuffix(r.URL.Path, "/embeddings"):
		width := payload.Dimensions
		if width == 0 {
			width = 8
		}
		vector := make([]float64, width)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"model":  payload.Model,
			"data":   []map[string]any{{"object": "embedding", "index": 0, "embedding": vector}},
			"usage":  map[string]int{"prompt_tokens": 1, "total_tokens": 1},
		})
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-stub",
			"object":  "chat.completion",
			"created": 0,
			"model":   payload.Model,
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": stubReply},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}
}

const stubReply = "testinfra-stub-reply"

func (s *stub) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
	s.failing = map[string]bool{}
}

func (s *stub) fail(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failing[model] = true
}

func (s *stub) seen() []stubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubRequest(nil), s.requests...)
}

func (s *stub) models() []string {
	out := []string{}
	for _, r := range s.seen() {
		out = append(out, r.model)
	}
	return out
}

var (
	gatewayOnce sync.Once
	gatewayURL  string
	gatewayErr  error
)

func TestMain(m *testing.M) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen for stub upstream: %v\n", err)
		os.Exit(1)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: upstream, ReadHeaderTimeout: 10 * time.Second},
	}
	server.Start()
	defer server.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve stub port: %v\n", err)
		os.Exit(1)
	}
	stubPort, err := strconv.Atoi(port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse stub port: %v\n", err)
		os.Exit(1)
	}

	gatewayOnce.Do(func() { gatewayURL, gatewayErr = testinfra.LiteLLM(context.Background(), stubPort) })

	os.Exit(m.Run())
}

func gatewayBaseURL(t *testing.T) string {
	t.Helper()
	if gatewayErr != nil {
		t.Fatalf("start litellm on gateway/config.yaml: %v", gatewayErr)
	}
	upstream.reset()
	return gatewayURL
}

func TestGatewayConfigLoadsInLiteLLM(t *testing.T) {
	base := gatewayBaseURL(t)
	cfg := loadGatewayConfig(t)

	body := getJSON(t, base+"/v1/models")

	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &models); err != nil {
		t.Fatalf("decode /v1/models: %v (body %s)", err, body)
	}
	served := make(map[string]bool, len(models.Data))
	for _, m := range models.Data {
		served[m.ID] = true
	}

	for name := range cfg.deployments() {
		if !served[name] {
			t.Errorf("group %q is declared in gateway/config.yaml but the running proxy does not serve it", name)
		}
	}

	for _, scenario := range requestedScenarioGroups {
		if !served[scenario] {
			t.Errorf("scenario %q is requested by the application but not served by the proxy", scenario)
		}
	}
}

func TestGatewayEnforcesMasterKey(t *testing.T) {
	base := gatewayBaseURL(t)

	req, err := http.NewRequest(http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := (&http.Client{Timeout: httpTimeout}).Do(req)
	if err != nil {
		t.Fatalf("GET /v1/models without a key: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status without a key = %d, want 401: the proxy is not enforcing its master key", resp.StatusCode)
	}
}

func TestGatewayRoutesScenarioToDeclaredPrimary(t *testing.T) {
	base := gatewayBaseURL(t)
	cfg := loadGatewayConfig(t)
	deployments := cfg.deployments()

	for _, scenario := range requestedScenarioGroups {
		if scenario == "embed" {
			continue
		}
		t.Run(scenario, func(t *testing.T) {
			upstream.reset()
			body := postJSON(t, base+"/v1/chat/completions", chatPayload(scenario), http.StatusOK)

			if content := chatContent(t, body); content != stubReply {
				t.Fatalf("content = %q, want %q: the request did not reach the stub upstream", content, stubReply)
			}

			seen := upstream.models()
			if len(seen) != 1 {
				t.Fatalf("upstream saw %d requests (%v), want exactly 1: the proxy retried or fell back on a healthy tier", len(seen), seen)
			}
			want := upstreamModel(t, deployments, fullChain(cfg, scenario)[0])
			if seen[0] != want {
				t.Fatalf("proxy asked upstream for %q, want %q — the running chain's first tier is not what the file declares", seen[0], want)
			}
		})
	}
}

func TestGatewayFallsBackToDeclaredTier(t *testing.T) {
	base := gatewayBaseURL(t)
	cfg := loadGatewayConfig(t)
	deployments := cfg.deployments()

	for _, scenario := range requestedScenarioGroups {
		chain := fullChain(cfg, scenario)
		if scenario == "embed" || len(chain) < 2 {
			continue
		}
		t.Run(scenario, func(t *testing.T) {
			upstream.reset()
			primary := upstreamModel(t, deployments, chain[0])
			fallback := upstreamModel(t, deployments, chain[1])
			upstream.fail(primary)

			body := postJSON(t, base+"/v1/chat/completions", chatPayload(scenario), http.StatusOK)
			if content := chatContent(t, body); content != stubReply {
				t.Fatalf("content = %q, want %q", content, stubReply)
			}

			seen := upstream.models()
			if len(seen) == 0 || seen[0] != primary {
				t.Fatalf("first upstream call was %v, want the primary tier %q", seen, primary)
			}
			if last := seen[len(seen)-1]; last != fallback {
				t.Fatalf("after failing %q the proxy ended on %q, want the declared fallback %q (full sequence %v)", primary, last, fallback, seen)
			}
		})
	}
}

func TestGatewayRejectsUnknownScenario(t *testing.T) {
	base := gatewayBaseURL(t)

	status, body := post(t, base+"/v1/chat/completions", chatPayload("no-such-scenario"))
	if status < 400 || status > 499 {
		t.Fatalf("status for an unknown scenario = %d, want 4xx (body %s)", status, truncate(body))
	}
	if seen := upstream.models(); len(seen) != 0 {
		t.Fatalf("an unknown scenario reached the upstream as %v: something is still serving as a catch-all", seen)
	}
}

func TestGatewayServesEmbedGroupAtDeclaredWidth(t *testing.T) {
	base := gatewayBaseURL(t)
	cfg := loadGatewayConfig(t)
	deployments := cfg.deployments()

	chain := fullChain(cfg, "embed")
	primary := deployments[chain[0]]
	wantModel := upstreamModel(t, deployments, chain[0])
	wantWidth, ok := embedWidth(primary)
	if !ok {
		t.Fatalf("embed tier %q declares no output_dimension", chain[0])
	}

	body := postJSON(t, base+"/v1/embeddings", map[string]any{
		"model": "embed",
		"input": "testinfra",
	}, http.StatusOK)

	var embeddings struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &embeddings); err != nil {
		t.Fatalf("decode /v1/embeddings: %v (body %s)", err, truncate(body))
	}
	if len(embeddings.Data) != 1 {
		t.Fatalf("got %d embeddings, want 1 (body %s)", len(embeddings.Data), truncate(body))
	}

	seen := upstream.seen()
	if len(seen) != 1 {
		t.Fatalf("upstream saw %d requests, want 1", len(seen))
	}
	if seen[0].model != wantModel {
		t.Fatalf("embed routed to %q, want the declared tier %q", seen[0].model, wantModel)
	}
	if seen[0].dimensions != wantWidth {
		t.Fatalf("embed requested %d dimensions, want the declared output_dimension %d — the width the schema is built on", seen[0].dimensions, wantWidth)
	}
}

func fullChain(c *gatewayConfig, scenario string) []string {
	return append([]string{scenario}, chainFor(c, scenario)...)
}

func upstreamModel(t *testing.T, deployments map[string]map[string]any, tier string) string {
	t.Helper()
	params, ok := deployments[tier]
	if !ok {
		t.Fatalf("tier %q is not declared in gateway/config.yaml", tier)
	}
	model, ok := params["model"].(string)
	if !ok {
		t.Fatalf("tier %q declares no model", tier)
	}
	_, rest, found := strings.Cut(model, "/")
	if !found {
		return model
	}
	return rest
}

func embedWidth(params map[string]any) (int, bool) {
	switch v := params["output_dimension"].(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func chatPayload(model string) map[string]any {
	return map[string]any{
		"model":    model,
		"messages": []map[string]string{{"role": "user", "content": "ping"}},
	}
}

func chatContent(t *testing.T, body []byte) string {
	t.Helper()
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &completion); err != nil {
		t.Fatalf("decode completion: %v (body %s)", err, truncate(body))
	}
	if len(completion.Choices) == 0 {
		t.Fatalf("no choices in completion (body %s)", truncate(body))
	}
	return completion.Choices[0].Message.Content
}

const httpTimeout = 120 * time.Second

func getJSON(t *testing.T, url string) []byte {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testinfra.LiteLLMMasterKey)

	resp, err := (&http.Client{Timeout: httpTimeout}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200 (body %s)", url, resp.StatusCode, truncate(body))
	}
	return body
}

func post(t *testing.T, url string, payload map[string]any) (int, []byte) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testinfra.LiteLLMMasterKey)

	resp, err := (&http.Client{Timeout: httpTimeout}).Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return resp.StatusCode, body
}

func postJSON(t *testing.T, url string, payload map[string]any, wantStatus int) []byte {
	t.Helper()
	status, body := post(t, url, payload)
	if status != wantStatus {
		t.Fatalf("POST %s = %d, want %d (body %s)", url, status, wantStatus, truncate(body))
	}
	return body
}

func truncate(body []byte) string {
	const max = 600
	if len(body) <= max {
		return string(body)
	}
	return fmt.Sprintf("%s… (%d bytes)", body[:max], len(body))
}
