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

// The guardrails in gateway_config_test.go read gateway/config.yaml as a YAML
// document. They cannot tell whether LiteLLM itself accepts it: a renamed
// key, a fallback shape the proxy no longer parses, or an `os.environ/*`
// reference that fails to resolve would pass every one of them and fail on
// `docker compose up`. These tests close that gap by running the pinned proxy
// image on the real file (internal/testinfra) and driving it over HTTP.
//
// No provider is contacted and no credential is needed: both provider base
// URLs point at the stub upstream below, so the proxy resolves scenarios,
// walks fallback chains and applies its own retry policy against fake models.
// What is under test is the routing contract, never a model.

// stub is an OpenAI-compatible upstream standing in for every provider. It
// records the model each request asked for — which is how a test tells which
// tier of a chain the proxy chose — and lets a test make a given model fail
// so the declared fallback is exercised.
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
		// 500 is retried and then failed over by the proxy; 4xx auth errors
		// are not, so this is the status that exercises the chain.
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

// reset clears the recorded requests and failure injections so each test
// reads only its own traffic. The proxy is shared by the whole package, so
// these tests do not run in parallel with each other.
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

// TestMain starts the stub on a fixed listener before any test runs, because
// the proxy container has to be told the port at creation time.
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

// TestGatewayConfigLoadsInLiteLLM proves gateway/config.yaml is a config the
// pinned proxy accepts, and that every deployment in the file became a group
// the running proxy serves. The container only reports ready once
// /health/liveliness answers, which LiteLLM does not do when config load
// failed — so reaching this test's body is already part of the assertion.
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
	// requestedScenarioGroups is what the application actually sends as
	// `model` (cmd/server/compose.go). Checked separately from the loop
	// above so a chain deleted from both file and application still fails.
	for _, scenario := range requestedScenarioGroups {
		if !served[scenario] {
			t.Errorf("scenario %q is requested by the application but not served by the proxy", scenario)
		}
	}
}

// TestGatewayEnforcesMasterKey proves master_key: os.environ/LITELLM_MASTER_KEY
// resolved. A config where it did not would serve an open proxy.
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

// TestGatewayRoutesScenarioToDeclaredPrimary sends one chat completion per
// scenario the application requests and asserts the proxy asked the upstream
// for the model gateway/config.yaml declares as that chain's first tier. This
// is the assertion the YAML guardrails cannot make: it is the running proxy's
// own resolution of the file, not a re-reading of it.
func TestGatewayRoutesScenarioToDeclaredPrimary(t *testing.T) {
	base := gatewayBaseURL(t)
	cfg := loadGatewayConfig(t)
	deployments := cfg.deployments()

	for _, scenario := range requestedScenarioGroups {
		if scenario == "embed" {
			continue // an embedding group; covered by its own test below
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

// TestGatewayFallsBackToDeclaredTier makes each chain's primary tier fail at
// the upstream and asserts the proxy reaches the tier the file names as its
// fallback. The `litellm_settings.fallbacks` shape is the part of the config
// most likely to break silently on an image bump: a fallback list the proxy
// stopped parsing does not fail config load, it just stops failing over.
func TestGatewayFallsBackToDeclaredTier(t *testing.T) {
	base := gatewayBaseURL(t)
	cfg := loadGatewayConfig(t)
	deployments := cfg.deployments()

	for _, scenario := range requestedScenarioGroups {
		chain := fullChain(cfg, scenario)
		if scenario == "embed" || len(chain) < 2 {
			continue // no fallback declared; nothing to fail over to
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

// TestGatewayRejectsUnknownScenario proves what gateway/config.yaml's header
// promises for 044: with the `default` catch-all group removed, a scenario
// name the file does not declare fails loudly instead of being served by
// something. A typo must reach the application as an error, not as an answer
// from an unintended model.
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

// TestGatewayServesEmbedGroupAtDeclaredWidth covers the one group the chat
// tests skip. `embed` is requested through /v1/embeddings by Router.Embed,
// and its output_dimension is the width the database's vector columns are
// declared at (C5-1), so this asserts the proxy both routes the group and
// passes that width downstream.
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

// fullChain is the ordered list of deployments a request for scenario walks:
// the group of that name is tier 1, and litellm_settings.fallbacks lists the
// rest (chainFor returns only the fallback tiers, not the primary).
func fullChain(c *gatewayConfig, scenario string) []string {
	return append([]string{scenario}, chainFor(c, scenario)...)
}

// upstreamModel strips the provider prefix LiteLLM routes on
// (openrouter/deepseek/deepseek-v4-pro) down to the model id it actually
// sends upstream (deepseek/deepseek-v4-pro), which is what the stub records.
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

// httpTimeout is generous because these assertions are about routing, not
// latency: a chain that retries then falls over takes several seconds even
// against an instant upstream.
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
