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

type ReplayFixture struct {
	RequestHash    string         `json:"request_hash"`
	RequestSummary RequestSummary `json:"request_summary"`
	Response       string         `json:"response"`
	RecordedAt     string         `json:"recorded_at"`
	RecordedFrom   string         `json:"recorded_from"`
}

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

type ReplayProvider struct {
	modelKey string
	dir      string

	mu       sync.Mutex
	fixtures map[string]ReplayFixture

	misses []RequestSummary

	recorded map[string]ReplayFixture

	record bool
	live   llm.Provider
}

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

func (p *ReplayProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (p *ReplayProvider) missCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.misses)
}

var _ llm.Provider = (*ReplayProvider)(nil)

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

	key2, msgs2, opts2 := base()
	if again, _ := hashRequest(key2, msgs2, opts2); again != original {
		t.Errorf("the same request hashed to %s then %s; replay would never match", original, again)
	}
}

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
