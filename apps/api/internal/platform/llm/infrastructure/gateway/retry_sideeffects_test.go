package gateway

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

type sideEffects struct {
	requests int32
	served   []string
	usages   []domain.Usage
}

func runStrictCall(t *testing.T, schema string, respond func(attempt int32, w http.ResponseWriter)) *sideEffects {
	t.Helper()
	fx := &sideEffects{}
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&fx.requests, 1)
		respond(n, w)
	})

	ctx := context.Background()
	ctx, servedPtr := domain.WithServedModelCapture(ctx)
	ctx, usagePtr := domain.WithUsageCapture(ctx)

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

func TestUnparseableSchemaSkipsTheRetry(t *testing.T) {
	fx := runStrictCall(t, "{not json", func(attempt int32, w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
	})
	if fx.requests != 1 {
		t.Errorf("upstream saw %d requests, want 1; a schema-parse failure already downgraded to json_object, so retrying would repeat an identical failing request", fx.requests)
	}
}

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
