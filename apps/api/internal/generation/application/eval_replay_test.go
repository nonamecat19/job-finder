package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/job-finder/api/internal/platform/llm"
)

// Replay (038 FR-010, contracts C3-1/C3-2).
//
// The deterministic gate must produce the same scores on any machine, with no
// credentials and no network. It does that by answering every model request
// from a committed fixture keyed by a hash over the request.
//
// **Deviation from the 038 contract, recorded rather than silently applied.**
// C3-1 specifies the hash covers "model key, prompt, System, Temperature,
// MaxTokens, ResponseMode and JSONSchema", and says explicitly: "There is no
// message list; do not hash one." That was true when 038 was written. Feature
// 037 has since landed, and CompleteStructured now routes through CompleteChat
// over a []Message — so the request genuinely *is* a message list, and hashing
// only a prompt string would key different conversations to the same fixture.
// The hash below therefore covers the messages, and the contract's intent — that
// every field which can change the response is in the key — is preserved
// exactly. TestReplayHashCoversEveryRequestField enforces it.

// ReplayFixture is one committed request/response pair.
type ReplayFixture struct {
	RequestHash    string         `json:"request_hash"`
	RequestSummary RequestSummary `json:"request_summary"`
	Response       string         `json:"response"`
	RecordedAt     string         `json:"recorded_at"`
	RecordedFrom   string         `json:"recorded_from"`
}

// RequestSummary is human-readable context for a miss. It is **not** part of
// the key: a failure message has to be readable, and a summary in the key would
// make trivial prompt edits look like hash changes for the wrong reason.
type RequestSummary struct {
	ModelKey     string `json:"model_key"`
	PromptPrefix string `json:"prompt_prefix"`
	PromptLen    int    `json:"prompt_len"`
	SystemLen    int    `json:"system_len"`
	Messages     int    `json:"messages"`
	Temperature  string `json:"temperature"`
	MaxTokens    string `json:"max_tokens"`
	ResponseMode int    `json:"response_mode"`
	SchemaLen    int    `json:"schema_len"`
	JSONOutput   bool   `json:"json_output"`
}

// requestKey is everything about a request that can change the response.
//
// Every field here was chosen because leaving it out would let a change alter
// the model's answer while every fixture still matched — which is a harness
// that reports "no change" for a change. The original 038 key omitted
// temperature and token caps, which meant raising selectMaxTokens would have
// invalidated no fixture at all.
type requestKey struct {
	ModelKey     string   `json:"model_key"`
	Roles        []string `json:"roles"`
	Contents     []string `json:"contents"`
	System       string   `json:"system"`
	Temperature  string   `json:"temperature"`
	MaxTokens    string   `json:"max_tokens"`
	ResponseMode int      `json:"response_mode"`
	JSONSchema   string   `json:"json_schema"`
	JSONOutput   bool     `json:"json_output"`
	ToolChoice   string   `json:"tool_choice"`
	Tools        []string `json:"tools"`
}

func hashRequest(modelKey string, msgs []llm.Message, opts *llm.CompleteOptions) (string, requestKey) {
	k := requestKey{ModelKey: modelKey}
	for _, m := range msgs {
		k.Roles = append(k.Roles, m.Role)
		k.Contents = append(k.Contents, m.Content)
	}
	if opts != nil {
		k.System = opts.System
		if opts.Temperature != nil {
			k.Temperature = fmt.Sprintf("%v", *opts.Temperature)
		}
		if opts.MaxTokens != nil {
			k.MaxTokens = fmt.Sprintf("%d", *opts.MaxTokens)
		}
		k.ResponseMode = int(opts.ResponseMode)
		k.JSONSchema = opts.JSONSchema
		k.JSONOutput = opts.JSONOutput
		k.ToolChoice = opts.ToolChoice
		for _, tool := range opts.Tools {
			k.Tools = append(k.Tools, tool.Name+"|"+tool.ArgsSchema)
		}
	}
	raw, err := json.Marshal(k)
	if err != nil {
		panic("eval: request key is not marshalable: " + err.Error())
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), k
}

func summarize(modelKey string, msgs []llm.Message, opts *llm.CompleteOptions, k requestKey) RequestSummary {
	s := RequestSummary{
		ModelKey:     modelKey,
		Messages:     len(msgs),
		SystemLen:    len(k.System),
		Temperature:  k.Temperature,
		MaxTokens:    k.MaxTokens,
		ResponseMode: k.ResponseMode,
		SchemaLen:    len(k.JSONSchema),
		JSONOutput:   k.JSONOutput,
	}
	if len(msgs) > 0 {
		last := msgs[len(msgs)-1].Content
		s.PromptLen = len(last)
		if len(last) > 160 {
			last = last[:160]
		}
		s.PromptPrefix = strings.ReplaceAll(last, "\n", " ")
	}
	return s
}

// ReplayProvider answers from committed fixtures and never contacts anything.
type ReplayProvider struct {
	// modelKey names which stage this provider stands in for, so two stages
	// sending the identical conversation still key to different fixtures.
	modelKey string
	dir      string

	mu       sync.Mutex
	fixtures map[string]ReplayFixture
	// misses accumulates unmatched requests so a failure can report all of
	// them at once rather than one per run.
	misses []RequestSummary
	// recorded is written by the live recorder (eval_live_test.go).
	recorded map[string]ReplayFixture
	// record switches the provider into recording mode, where a miss delegates
	// to the live provider instead of failing. Never set in the gate: the
	// recorder is behind a build tag and is not compiled into that binary.
	record bool
	live   llm.Provider
}

// newReplayProvider loads a case's fixtures.
func newReplayProvider(t *testing.T, modelKey, caseName string) *ReplayProvider {
	t.Helper()
	p := &ReplayProvider{
		modelKey: modelKey,
		dir:      caseReplayDir(caseName),
		fixtures: map[string]ReplayFixture{},
		recorded: map[string]ReplayFixture{},
	}
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return p
		}
		t.Fatalf("read replays for %s: %v", caseName, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(p.dir, e.Name()))
		if err != nil {
			t.Fatalf("read fixture %s: %v", e.Name(), err)
		}
		var f ReplayFixture
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("parse fixture %s: %v", e.Name(), err)
		}
		p.fixtures[f.RequestHash] = f
	}
	return p
}

func (p *ReplayProvider) ModelName() string { return p.modelKey }

func (p *ReplayProvider) CompleteChat(ctx context.Context, msgs []llm.Message, opts *llm.CompleteOptions) (llm.ChatResult, error) {
	hash, key := hashRequest(p.modelKey, msgs, opts)

	p.mu.Lock()
	f, ok := p.fixtures[hash]
	p.mu.Unlock()
	if ok {
		return llm.ChatResult{Content: f.Response}, nil
	}

	if p.record && p.live != nil {
		res, err := p.live.CompleteChat(ctx, msgs, opts)
		if err != nil {
			return llm.ChatResult{}, err
		}
		p.mu.Lock()
		p.recorded[hash] = ReplayFixture{
			RequestHash:    hash,
			RequestSummary: summarize(p.modelKey, msgs, opts, key),
			Response:       res.Content,
		}
		p.mu.Unlock()
		return res, nil
	}

	// FR-010 / C3-2. A miss is loud, names the case and the request, and never
	// falls through: not to a live call, not to a default, not to the nearest
	// fixture. Each of those would turn "the corpus no longer describes this
	// code" into a quiet pass — which is the one outcome a gate must never
	// produce.
	summary := summarize(p.modelKey, msgs, opts, key)
	p.mu.Lock()
	p.misses = append(p.misses, summary)
	p.mu.Unlock()
	return llm.ChatResult{}, fmt.Errorf(
		"eval replay: no fixture for %s request %s\n"+
			"  stage=%s messages=%d prompt_len=%d system_len=%d temp=%s max_tokens=%s response_mode=%d schema_len=%d\n"+
			"  prompt starts: %s\n"+
			"  The request changed, so the recorded response no longer describes it. Re-record with:\n"+
			"    go test -tags eval_live ./internal/generation/application/ -run TestEvalRecord -eval.record -eval.case <case>",
		p.dir, hash, summary.ModelKey, summary.Messages, summary.PromptLen, summary.SystemLen,
		summary.Temperature, summary.MaxTokens, summary.ResponseMode, summary.SchemaLen, summary.PromptPrefix)
}

func (p *ReplayProvider) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	res, err := p.CompleteChat(ctx, llm.PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.3, false))
	return res.Content, err
}

func (p *ReplayProvider) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	res, err := p.CompleteChat(ctx, llm.PromptMessages(opts.SystemPrompt(), prompt), opts.ShimOptions(0.1, true))
	return res.Content, err
}

// Embed never touches a fixture. Embeddings do not go through the gateway at
// all (030-FR-014) and the tailoring path does not use them; returning a fixed
// vector keeps the interface satisfied without pretending to replay something
// that never happens here.
func (p *ReplayProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (p *ReplayProvider) missCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.misses)
}

var _ llm.Provider = (*ReplayProvider)(nil)

// --- tripwire ---------------------------------------------------------------

// TestReplayHashCoversEveryRequestField is a tripwire, not coverage (FR-029,
// C3-1a).
//
// It perturbs each hashed field in turn and asserts **every** perturbation
// misses. A field missing from the key does not make anything fail — it makes a
// changed request quietly match an old fixture, so the harness reports "no
// change" for a change and the gate passes while measuring the wrong thing.
//
// This test would have caught the original 038 key, which omitted temperature
// and MaxTokens: raising selectMaxTokens would have invalidated no fixture.
func TestReplayHashCoversEveryRequestField(t *testing.T) {
	temp := 0.1
	tokens := 8000
	base := func() (string, []llm.Message, *llm.CompleteOptions) {
		return "generation-select",
			[]llm.Message{
				{Role: "system", Content: "you tailor resumes"},
				{Role: "user", Content: "tailor this"},
			},
			&llm.CompleteOptions{
				System:       "you tailor resumes",
				Temperature:  &temp,
				MaxTokens:    &tokens,
				ResponseMode: llm.ResponseModeStrict,
				JSONSchema:   `{"type":"object"}`,
				JSONOutput:   true,
			}
	}

	baseKey, _, _ := base()
	baseMsgs, baseOpts := func() ([]llm.Message, *llm.CompleteOptions) {
		_, m, o := base()
		return m, o
	}()
	original, _ := hashRequest(baseKey, baseMsgs, baseOpts)

	otherTemp := 0.9
	otherTokens := 4000

	perturbations := map[string]func(key *string, msgs *[]llm.Message, opts *llm.CompleteOptions){
		"model key": func(key *string, _ *[]llm.Message, _ *llm.CompleteOptions) {
			*key = "generation-summary"
		},
		"prompt content": func(_ *string, msgs *[]llm.Message, _ *llm.CompleteOptions) {
			(*msgs)[1].Content = "tailor this differently"
		},
		"message role": func(_ *string, msgs *[]llm.Message, _ *llm.CompleteOptions) {
			(*msgs)[0].Role = "user"
		},
		"an extra turn": func(_ *string, msgs *[]llm.Message, _ *llm.CompleteOptions) {
			*msgs = append(*msgs, llm.Message{Role: "assistant", Content: "ok"})
		},
		"system prompt": func(_ *string, _ *[]llm.Message, opts *llm.CompleteOptions) {
			opts.System = "you write cover letters"
		},
		"temperature": func(_ *string, _ *[]llm.Message, opts *llm.CompleteOptions) {
			opts.Temperature = &otherTemp
		},
		"max tokens": func(_ *string, _ *[]llm.Message, opts *llm.CompleteOptions) {
			opts.MaxTokens = &otherTokens
		},
		"response mode": func(_ *string, _ *[]llm.Message, opts *llm.CompleteOptions) {
			opts.ResponseMode = llm.ResponseModeJSON
		},
		"json schema": func(_ *string, _ *[]llm.Message, opts *llm.CompleteOptions) {
			opts.JSONSchema = `{"type":"object","properties":{"a":{"type":"string"}}}`
		},
		"json output": func(_ *string, _ *[]llm.Message, opts *llm.CompleteOptions) {
			opts.JSONOutput = false
		},
		"tool choice": func(_ *string, _ *[]llm.Message, opts *llm.CompleteOptions) {
			opts.ToolChoice = "required"
		},
	}

	names := make([]string, 0, len(perturbations))
	for name := range perturbations {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			key, msgs, opts := base()
			// Deep-copy the message slice so a perturbation cannot leak.
			cp := make([]llm.Message, len(msgs))
			copy(cp, msgs)
			perturbations[name](&key, &cp, opts)

			got, _ := hashRequest(key, cp, opts)
			if got == original {
				t.Errorf("changing %s did not change the request hash.\n"+
					"A request that changes while its fixture still matches makes the harness report "+
					"'no change' for a change — the gate passes while measuring the wrong thing.", name)
			}
		})
	}

	// The converse: an identical request must hash identically, or nothing ever
	// matches and every run is a miss.
	key2, msgs2, opts2 := base()
	if again, _ := hashRequest(key2, msgs2, opts2); again != original {
		t.Errorf("the same request hashed to %s then %s; replay would never match", original, again)
	}
}

// A miss must fail loudly and name what it could not find. Asserted because the
// tempting implementations of "no fixture" — return empty, return the nearest,
// call the live provider — all turn a stale corpus into a quiet pass.
func TestReplayMissFailsLoudlyAndNeverFallsThrough(t *testing.T) {
	p := &ReplayProvider{modelKey: "generation-select", dir: "evaldata/replays/nonexistent", fixtures: map[string]ReplayFixture{}}

	res, err := p.CompleteChat(context.Background(), []llm.Message{{Role: "user", Content: "unrecorded"}}, nil)
	if err == nil {
		t.Fatal("a request with no fixture returned no error; a stale corpus would pass silently")
	}
	if res.Content != "" {
		t.Errorf("a miss returned content %q; it must return nothing", res.Content)
	}
	for _, want := range []string{"no fixture", "generation-select", "eval.record"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure message does not contain %q, so a reader cannot act on it:\n%s", want, err)
		}
	}
	if p.missCount() != 1 {
		t.Errorf("miss count = %d, want 1", p.missCount())
	}
}
