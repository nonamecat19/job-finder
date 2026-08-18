//go:build integration

package aicontract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/events"
	ghostapp "github.com/job-finder/api/internal/ghostjob/application"
	ghostdomain "github.com/job-finder/api/internal/ghostjob/domain"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/testinfra"
)

func testBrokerURL(t *testing.T) string {
	t.Helper()
	url, err := testinfra.RabbitMQURL(context.Background())
	if err != nil {
		t.Fatalf("start rabbitmq container: %v", err)
	}
	return url
}

func dialTestBroker(t *testing.T) *amqp.Connection {
	t.Helper()
	url := testBrokerURL(t)
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(10 * time.Second)})
	if err != nil {
		t.Fatalf("dial broker at %s: %v", url, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func declareTopologyOrFail(t *testing.T, conn *amqp.Connection) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel for topology: %v", err)
	}
	defer ch.Close()
	if err := events.DeclareTopology(ch); err != nil {
		t.Fatalf("declare topology: %v", err)
	}
}

func newTestPublisher(t *testing.T, conn *amqp.Connection) (*events.Publisher, *amqp.Channel) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open publisher channel: %v", err)
	}
	pub, err := events.NewPublisher(ch, 5*time.Second)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	return pub, ch
}

func purgeQueue(t *testing.T, conn *amqp.Connection, queue string) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel to purge %s: %v", queue, err)
	}
	defer ch.Close()
	if _, err := ch.QueuePurge(queue, false); err != nil {
		t.Fatalf("purge %s: %v", queue, err)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}

func publishWork(t *testing.T, pub *events.Publisher, workType string, body []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	headers := amqp.Table{
		events.HeaderAttempt:  int32(0),
		events.HeaderWorkType: workType,
	}
	if err := pub.Publish(ctx, events.WorkExchange, workType, body, headers); err != nil {
		t.Fatalf("publish work to %s: %v", workType, err)
	}
}

const (
	aiUser = "ai_service"
	aiPass = "testinfra-ai-secret"

	aiServiceToken = "testinfra-ai-service-token"

	stubScore       = 82.5
	stubConfidence  = 0.95
	stubExplanation = "reposted four times with no candidate ever hired"

	stubPromptTokens     = 137
	stubCompletionTokens = 41
)

var stubTopSignals = []string{"repost_count", "always_hiring"}

type ghostGateway struct {
	mu     sync.Mutex
	models []string

	reply string
}

func (g *ghostGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Model string `json:"model"`
	}
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &payload)

	g.mu.Lock()
	g.models = append(g.models, payload.Model)
	reply := g.reply
	g.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      "chatcmpl-ghost-stub",
		"object":  "chat.completion",
		"created": 0,
		"model":   payload.Model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]string{"role": "assistant", "content": reply},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{
			"prompt_tokens":     stubPromptTokens,
			"completion_tokens": stubCompletionTokens,
			"total_tokens":      stubPromptTokens + stubCompletionTokens,
		},
	})
}

func (g *ghostGateway) seen() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.models...)
}

var (
	gatewayOnce sync.Once
	gatewayStub *ghostGateway
	gatewayErr  error
)

func startAIService(t *testing.T) *ghostGateway {
	t.Helper()
	if err := startAIServiceErr(); err != nil {
		t.Fatal(err)
	}
	return gatewayStub
}

func startAIServiceErr() error {
	gatewayOnce.Do(func() {

		reply, err := json.Marshal(map[string]any{
			"score":       stubScore,
			"confidence":  stubConfidence,
			"explanation": stubExplanation,
			"topSignals":  stubTopSignals,
		})
		if err != nil {
			gatewayErr = fmt.Errorf("marshal stub model reply: %w", err)
			return
		}

		gatewayStub = &ghostGateway{reply: string(reply)}

		server := httptest.NewServer(gatewayStub)

		_, portText, err := net.SplitHostPort(server.Listener.Addr().String())
		if err != nil {
			gatewayErr = fmt.Errorf("split stub gateway address %q: %w", server.URL, err)
			return
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			gatewayErr = fmt.Errorf("parse stub gateway port %q: %w", portText, err)
			return
		}

		if err := testinfra.ProvisionRabbitMQAIUser(context.Background(), aiUser, aiPass); err != nil {
			gatewayErr = fmt.Errorf("run init-ai-user.sh: %w", err)
			return
		}

		if _, err := testinfra.AIService(context.Background(), testinfra.AIServiceConfig{
			BrokerUser:   aiUser,
			BrokerPass:   aiPass,
			GatewayPort:  port,
			ServiceToken: aiServiceToken,
		}); err != nil {
			gatewayErr = fmt.Errorf("build and start the apps/ai service as %s: %w", aiUser, err)
		}
	})
	return gatewayErr
}

type ghostRequestedMessage struct {
	events.Envelope
	events.GhostWork
}

func newGhostRequest(t *testing.T) (ghostRequestedMessage, []byte) {
	t.Helper()

	jobID := uuid.NewString()
	daysOpen := 96
	snapshot := ghostapp.GhostSnapshot{
		JobID:       jobID,
		Title:       "Senior Backend Engineer",
		Company:     "Perpetual Hiring Co",
		RepostCount: 4,
		DaysOpen:    &daysOpen,

		Notes: map[string]string{"source": "wire-contract-test"},
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	sum := sha256.Sum256(snapshotJSON)

	correlationID := uuid.NewString()
	msg := ghostRequestedMessage{
		Envelope: events.Envelope{
			EventID:        uuid.NewString(),
			EventType:      events.EventGhostRequested,
			SchemaVersion:  1,
			OccurredAt:     time.Now().UTC().Truncate(time.Millisecond),
			WorkID:         jobID,
			CorrelationID:  correlationID,
			IdempotencyKey: fmt.Sprintf("ghost:%s:%s", jobID, correlationID),
			RunID:          uuid.NewString(),
		},
		GhostWork: events.GhostWork{
			GhostScorePayload: queue.GhostScorePayload{JobID: jobID},
			Snapshot:          events.InputSnapshot(snapshotJSON),
			SnapshotHash:      "sha256:" + hex.EncodeToString(sum[:]),
		},
	}
	return msg, mustMarshal(t, msg)
}

type completedResult struct {
	envelope events.Envelope
	result   events.Result
}

func startResultConsumer(t *testing.T) <-chan completedResult {
	t.Helper()

	results := make(chan completedResult, 8)
	registry := events.ResultRegistry{
		events.EventGhostCompleted: func(_ context.Context, env events.Envelope, res events.Result) error {
			results <- completedResult{envelope: env, result: res}
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	consumer := &events.Consumer{
		Dial:        func() (*amqp.Connection, error) { return amqp.Dial(testBrokerURL(t)) },
		Queue:       events.ResultQueue,
		Concurrency: 1,
		HandlerFunc: registry.HandleResultDelivery(slog.Default()),
	}
	done := make(chan error, 1)
	go func() { done <- consumer.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return results
}

func awaitResult(t *testing.T, results <-chan completedResult, what string) completedResult {
	t.Helper()
	select {
	case got := <-results:
		return got
	case <-time.After(90 * time.Second):
		t.Fatalf("no ghost.completed arrived on %s within 90s (%s) — the Python service either never consumed the work, failed to parse it, or published a result the Go consumer discarded as malformed", events.ResultQueue, what)
		return completedResult{}
	}
}

func TestGhostPython_Integration_ServiceWaitsForTopologyItCannotDeclare(t *testing.T) {
	conn := dialTestBroker(t)
	assertTopologyAbsent(t, conn)

	started := make(chan error, 1)
	go func() {

		defer func() {
			if r := recover(); r != nil {
				started <- fmt.Errorf("starting the ai service panicked: %v", r)
			}
		}()
		started <- startAIServiceErr()
	}()

	time.Sleep(topologyGap)

	declareTopologyOrFail(t, conn)

	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("the ai service did not survive being started before the backend declared topology: %v", err)
		}
	case <-time.After(3 * time.Minute):
		t.Fatal("the ai service never became ready after topology appeared — wait_for_topology is not converging")
	}

}

const topologyGap = 8 * time.Second

func assertTopologyAbsent(t *testing.T, conn *amqp.Connection) {
	t.Helper()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel to probe topology: %v", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclarePassive(events.WorkExchange, "direct", true, false, false, false, nil); err == nil {
		t.Fatalf("%s already exists — this test only means something on a broker nobody has declared topology on, so it must run before every other test in this package", events.WorkExchange)
	}
}

func TestGhostPython_Integration_RealPythonServiceRoundTripsGhostWork(t *testing.T) {

	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)
	purgeQueue(t, conn, "work.ghost")
	purgeQueue(t, conn, "dlq.ghost")
	purgeQueue(t, conn, events.ResultQueue)

	gateway := startAIService(t)

	results := startResultConsumer(t)
	pub, pubCh := newTestPublisher(t, conn)
	defer pubCh.Close()

	request, body := newGhostRequest(t)
	publishWork(t, pub, "ghost", body)

	got := awaitResult(t, results, "the first ghost.requested event")

	if got.envelope.EventType != events.EventGhostCompleted {
		t.Fatalf("result event_type = %q, want %q", got.envelope.EventType, events.EventGhostCompleted)
	}
	if got.envelope.SchemaVersion != 1 {
		t.Fatalf("result schema_version = %d, want 1 (HandleResultDelivery discards anything else)", got.envelope.SchemaVersion)
	}
	if got.envelope.WorkID != request.WorkID {
		t.Fatalf("result work_id = %q, want %q", got.envelope.WorkID, request.WorkID)
	}
	if got.envelope.CorrelationID != request.CorrelationID {
		t.Fatalf("result correlation_id = %q, want %q", got.envelope.CorrelationID, request.CorrelationID)
	}
	if got.envelope.IdempotencyKey != request.IdempotencyKey {
		t.Fatalf("result idempotency_key = %q, want %q", got.envelope.IdempotencyKey, request.IdempotencyKey)
	}
	if got.envelope.RunID != request.RunID {
		t.Fatalf("result run_id = %q, want %q", got.envelope.RunID, request.RunID)
	}
	if got.envelope.EventID == "" || got.envelope.EventID == request.EventID {
		t.Fatalf("result event_id = %q, want a fresh id distinct from the request's %q", got.envelope.EventID, request.EventID)
	}
	if got.envelope.OccurredAt.IsZero() {
		t.Fatal("result occurred_at did not parse into Go's time.Time — Python's datetime encoding and Go's RFC3339 expectation have diverged")
	}

	if got.result.Status != events.ResultSucceeded {
		t.Fatalf("result status = %q with failure %+v, want succeeded — the capability ran against a stub gateway that answers every call correctly", got.result.Status, got.result.Failure)
	}
	if got.result.SnapshotHash != request.SnapshotHash {
		t.Fatalf("result snapshot_hash = %q, want the request's %q — a result that cannot be tied back to the exact input it scored is not usable for cache invalidation", got.result.SnapshotHash, request.SnapshotHash)
	}

	var scored ghostdomain.GhostJobResult
	if err := json.Unmarshal(got.result.Result, &scored); err != nil {
		t.Fatalf("decode result into ghostdomain.GhostJobResult: %v (body %s)", err, got.result.Result)
	}
	if err := scored.Validate(); err != nil {
		t.Fatalf("the Python result fails the Go domain's own validation: %v", err)
	}
	if scored.Score != stubScore {
		t.Fatalf("score = %v, want %v (the stub model's exact answer, carried through unchanged)", scored.Score, stubScore)
	}
	if scored.Explanation != stubExplanation {
		t.Fatalf("explanation = %q, want %q", scored.Explanation, stubExplanation)
	}
	if len(scored.TopSignals) != len(stubTopSignals) {
		t.Fatalf("topSignals = %v, want %v — the camelCase key survives the Pydantic round trip only because ghost.py declares it that way", scored.TopSignals, stubTopSignals)
	}

	if scored.Confidence != 0.6 {
		t.Fatalf("confidence = %v, want 0.6 — ghost.py must cap confidence when the snapshot has unknown signals (the stub answered %v)", scored.Confidence, stubConfidence)
	}

	if got.result.Usage == nil {
		t.Fatal("result carried no usage — token counts must survive the gateway response, the capability and the wire, or spend is invisible to the backend")
	}
	if got.result.Usage.InputTokens == nil || *got.result.Usage.InputTokens != stubPromptTokens {
		t.Fatalf("usage.input_tokens = %v, want %d", got.result.Usage.InputTokens, stubPromptTokens)
	}
	if got.result.Usage.OutputTokens == nil || *got.result.Usage.OutputTokens != stubCompletionTokens {
		t.Fatalf("usage.output_tokens = %v, want %d", got.result.Usage.OutputTokens, stubCompletionTokens)
	}

	models := gateway.seen()
	if len(models) == 0 {
		t.Fatal("the stub gateway was never called — the result was produced without a model call at all")
	}
	for _, model := range models {
		if model != "ghost" {
			t.Fatalf("the service asked the gateway for model %q, want the task key \"ghost\"", model)
		}
	}

	assertQueueDrained(t, conn, "work.ghost", "after the Python service completed the work")
}

func TestGhostPython_Integration_MalformedWorkDoesNotWedgeTheConsumer(t *testing.T) {
	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)
	purgeQueue(t, conn, "work.ghost")
	purgeQueue(t, conn, "dlq.ghost")
	purgeQueue(t, conn, events.ResultQueue)

	startAIService(t)

	results := startResultConsumer(t)
	pub, pubCh := newTestPublisher(t, conn)
	defer pubCh.Close()

	poison := map[string]any{
		"event_id":        uuid.NewString(),
		"event_type":      "ghost.requested",
		"schema_version":  1,
		"occurred_at":     time.Now().UTC().Format(time.RFC3339Nano),
		"work_id":         uuid.NewString(),
		"correlation_id":  uuid.NewString(),
		"idempotency_key": "ghost:poison",
		"run_id":          uuid.NewString(),
		"unexpected":      "a field no generated model knows",
	}
	publishWork(t, pub, "ghost", mustMarshal(t, poison))

	request, body := newGhostRequest(t)
	publishWork(t, pub, "ghost", body)

	got := awaitResult(t, results, "the valid event published behind an unparseable one")
	if got.envelope.CorrelationID != request.CorrelationID {
		t.Fatalf("result correlation_id = %q, want the valid request's %q", got.envelope.CorrelationID, request.CorrelationID)
	}
	if got.result.Status != events.ResultSucceeded {
		t.Fatalf("result status = %q, want succeeded", got.result.Status)
	}

	assertQueueDrained(t, conn, "work.ghost", "after an unparseable message was rejected")

	assertQueueDepth(t, conn, "dlq.ghost", 1, "the unparseable message must be dead-lettered, not discarded")
}

func assertQueueDrained(t *testing.T, conn *amqp.Connection, queueName, why string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	var last int
	for time.Now().Before(deadline) {
		ch, err := conn.Channel()
		if err != nil {
			t.Fatalf("open channel to inspect %s: %v", queueName, err)
		}

		q, err := ch.QueueDeclarePassive(queueName, true, false, false, false, nil)
		ch.Close()
		if err != nil {
			t.Fatalf("inspect %s: %v", queueName, err)
		}
		last = q.Messages
		if last == 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s still holds %d message(s) %s — work is being redelivered rather than acked, or the consumer is wedged", queueName, last, why)
}

func assertQueueDepth(t *testing.T, conn *amqp.Connection, queueName string, want int, why string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	var last int
	for time.Now().Before(deadline) {
		ch, err := conn.Channel()
		if err != nil {
			t.Fatalf("open channel to inspect %s: %v", queueName, err)
		}

		q, err := ch.QueueDeclarePassive(queueName, true, false, false, false, nil)
		ch.Close()
		if err != nil {
			t.Fatalf("inspect %s: %v", queueName, err)
		}
		last = q.Messages
		if last == want {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s holds %d message(s), want %d — %s", queueName, last, want, why)
}
