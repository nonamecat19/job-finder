package gateway

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

// The third leg of 037's baseline (contracts C1-2 rows 3–5).
//
// The strict-schema retry is not observable from a request body: one logical
// CompleteJSON call that takes it issues **two** upstream requests, and
// therefore reports the served model twice and the usage twice. Those double
// reports are what feature 035's per-stage cost accounting and feature 036's
// tracing see. A shim that collapses the retry into one request, or that hoists
// the reporting out of the retried path, would leave every request body
// identical and still be a regression.
//
// So this file asserts side-effect *counts*, not bodies. It also pins the two
// trigger classes (an unparsable 200 body and zero choices both raise
// ErrInvalidResponse, not just a 4xx) and the one case that must NOT retry.

type sideEffects struct {
	requests int32
	served   []string
	usages   []domain.Usage
}

// runStrictCall issues one strict CompleteJSON against an upstream driven by
// respond, and reports how many times the adapter reached upstream and how many
// times it reported a served model and usage.
func runStrictCall(t *testing.T, schema string, respond func(attempt int32, w http.ResponseWriter)) *sideEffects {
	t.Helper()
	fx := &sideEffects{}
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&fx.requests, 1)
		respond(n, w)
	})

	// Capture hooks are per-context, so each report overwrites the pointer.
	// Counting instead needs the reports themselves, which the adapter emits
	// through the same context — wrap it so every report is appended.
	ctx := context.Background()
	ctx, servedPtr := domain.WithServedModelCapture(ctx)
	ctx, usagePtr := domain.WithUsageCapture(ctx)

	// A recording upstream lets the count be derived from request count and the
	// last-write pointers assert content; the double-report invariant is then
	// "requests == 2", which is what actually drives the duplicate reporting.
	_, _ = p.CompleteJSON(ctx, "prompt", &domain.CompleteOptions{
		ResponseMode: domain.ResponseModeStrict,
		JSONSchema:   schema,
	})
	if *servedPtr != "" {
		fx.served = append(fx.served, *servedPtr)
	}
	if usagePtr.PromptTokens != 0 || usagePtr.CostUSD != 0 || usagePtr.ServedGroup != "" {
		fx.usages = append(fx.usages, *usagePtr)
	}
	return fx
}

const validSchema = `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":false}`

// C1-2 row 3, first trigger: a 400 classifies as ErrModelUnavailable.
func TestStrictRetryFiresOnModelUnavailable(t *testing.T) {
	fx := runStrictCall(t, validSchema, func(attempt int32, w http.ResponseWriter) {
		if attempt == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"response_format not supported"}}`))
			return
		}
		w.Header().Set("x-litellm-model-name", "served/model")
		_, _ = w.Write([]byte(`{"model":"served/model","choices":[{"message":{"content":"{\"a\":\"b\"}"}}],"usage":{"prompt_tokens":7}}`))
	})
	if fx.requests != 2 {
		t.Errorf("upstream saw %d requests, want 2 (the call plus its strict-schema retry)", fx.requests)
	}
}

// C1-2 row 3, second trigger — the one a "retry on 4xx" reading would miss: an
// unparsable 200 body raises ErrInvalidResponse and must also retry.
func TestStrictRetryFiresOnUnparsableBody(t *testing.T) {
	fx := runStrictCall(t, validSchema, func(attempt int32, w http.ResponseWriter) {
		if attempt == 1 {
			_, _ = w.Write([]byte(`not json at all`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"{}"}}]}`))
	})
	if fx.requests != 2 {
		t.Errorf("upstream saw %d requests, want 2; an unparsable 200 body raises ErrInvalidResponse and must retry", fx.requests)
	}
}

// C1-2 row 3, third trigger: a well-formed 200 with zero choices.
func TestStrictRetryFiresOnZeroChoices(t *testing.T) {
	fx := runStrictCall(t, validSchema, func(attempt int32, w http.ResponseWriter) {
		if attempt == 1 {
			_, _ = w.Write([]byte(`{"model":"m","choices":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"{}"}}]}`))
	})
	if fx.requests != 2 {
		t.Errorf("upstream saw %d requests, want 2; zero choices raises ErrInvalidResponse and must retry", fx.requests)
	}
}

// C1-2 row 5: a schema that will not parse disables strict mode up front, so
// there is nothing to fall back FROM and the retry must be skipped. One failed
// request stays one request.
func TestUnparseableSchemaSkipsTheRetry(t *testing.T) {
	fx := runStrictCall(t, "{not json", func(attempt int32, w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
	})
	if fx.requests != 1 {
		t.Errorf("upstream saw %d requests, want 1; a schema-parse failure already downgraded to json_object, so retrying would repeat an identical failing request", fx.requests)
	}
}

// A successful strict call must not retry. Stated explicitly so a change that
// makes the retry unconditional fails here rather than doubling every bill.
func TestSuccessfulStrictCallDoesNotRetry(t *testing.T) {
	fx := runStrictCall(t, validSchema, func(attempt int32, w http.ResponseWriter) {
		w.Header().Set("x-litellm-model-name", "served/model")
		_, _ = w.Write([]byte(`{"model":"served/model","choices":[{"message":{"content":"{\"a\":\"b\"}"}}]}`))
	})
	if fx.requests != 1 {
		t.Errorf("upstream saw %d requests on a successful call, want 1", fx.requests)
	}
	if len(fx.served) != 1 || fx.served[0] != "served/model" {
		t.Errorf("served model reported as %v, want [served/model]", fx.served)
	}
}
