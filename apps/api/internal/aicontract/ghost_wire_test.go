//go:build integration

// Package aicontract holds the one test that runs the Go backend and the real
// Python AI service against each other.
//
// It is its own package, and therefore its own test binary, because the AI
// service container consumes work.ghost, work.match, work.generate and
// work.salary for as long as the binary lives (internal/testinfra starts each
// container once per process). Inside internal/events that container would
// silently steal the deliveries redelivery_test.go and friends publish to
// those same fixed queues — topology.go's WorkTypes is a closed set, so there
// are no per-test queue names to isolate behind.
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

// This is the only place the Go backend and the *real* Python AI service
// meet. Everything else about the ghost hand-off is tested against a stand-in:
// internal/events/ghost_python_durability_test.go publishes ghost.requested
// and then consumes it with a Go consumer, which proves broker mechanics and
// proves nothing at all about apps/ai.
//
// What is genuinely at risk is the wire agreement described in
// apps/ai/src/jobfinder_ai/messaging/consumers.py's module docstring: a work
// or result body is the envelope's fields merged FLAT with the payload's
// fields, not `{"envelope": …, "payload": …}`. Nothing in either build
// enforces that. Go could start nesting (embed a named field instead of an
// anonymous one), Python could start expecting nesting, a generated Pydantic
// model could gain a required field, `extra="forbid"` could start rejecting a
// field Go added — and every existing test on both sides would still pass,
// because each side only ever talks to a mirror of itself.
//
// So this test runs the artifact that ships: apps/ai's image, built from
// apps/ai/Dockerfile with the repository root as context exactly as
// docker-compose.yml builds it, against the real broker, with topology
// declared from the Go side and only from there (M1-1). The Go publisher
// publishes; the Python consumer parses it, runs the capability and publishes
// back; the Go result consumer decodes what came back through
// events.ResultRegistry — the same code path cmd/server uses. A disagreement
// anywhere in that loop fails here.
//
// No provider is contacted. GATEWAY_URL points at ghostGateway below, an
// OpenAI-compatible stub served from this test process and reached through
// testcontainers' host-port tunnel, so the "model" is a fixture whose exact
// bytes this test chose.
//
// The service runs as the restricted `ai_service` account
// docker/rabbitmq/init-ai-user.sh creates — provisioned here by running that
// very script — because that is what compose runs it as, and because the
// account is load-bearing: it has `"configure":"^$"`, so the AI service cannot
// declare, alter or delete any topology (M1-1, M7-3).
//
// That combination is a regression test for a defect this file found. The
// service used to build its RabbitQueue/RabbitExchange objects with
// FastStream's default declare=True, which made it declare both exchanges and
// all four work queues at startup; under the account compose gives it that is
// ACCESS_REFUSED and the process exits, so the shipped stack's AI service
// crash-looped on boot and nothing caught it — apps/ai's own broker tests
// connect as an administrator. consumers.py now passes declare=False
// throughout, which FastStream turns into a passive declaration: it asserts
// the backend's topology rather than creating a second opinion of it, and a
// passive declare is permitted on a resource the account holds any permission
// on (the work queues by the read regex, jobfinder.results by the write one).
// Reintroduce an active declare and this file's container never reaches
// /health/ready, so every test here fails.

// ---------------------------------------------------------------------------
// Broker helpers. These mirror internal/events/helpers_test.go rather than
// importing it — test helpers are not importable across packages, and this
// package needs only the handful below.

// testBrokerURL is the URL of the RabbitMQ container this test binary owns.
func testBrokerURL(t *testing.T) string {
	t.Helper()
	url, err := testinfra.RabbitMQURL(context.Background())
	if err != nil {
		t.Fatalf("start rabbitmq container: %v", err)
	}
	return url
}

// dialTestBroker opens an administrator connection to the container broker.
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

// declareTopologyOrFail declares the full topology from the Go side, which is
// the publisher's job and nobody else's (M1-1).
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

// newTestPublisher opens a fresh channel and wraps it in the real
// events.Publisher, confirms and all.
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

// purgeQueue empties a queue before a test runs. The per-work-type queues are
// a fixed, closed set, so a leftover from a previous run would otherwise be
// consumed as if this run had published it.
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

// publishWork publishes body to the work exchange under workType's routing
// key with the headers a real dispatch carries, waiting for the broker's
// publish confirm (M2-1).
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
	// The restricted broker account compose gives the AI service, created by
	// docker/rabbitmq/init-ai-user.sh. The name matters: the script's read
	// permission regex is scoped to the AI work queues by queue name, not by
	// account name, but RABBITMQ_AI_USER is what compose defaults to.
	aiUser = "ai_service"
	aiPass = "testinfra-ai-secret"

	// The shared secret the service boots with. It is unused by the event
	// transport (that surface is HTTP-only), but settings.REQUIRED_ENV_KEYS
	// makes it mandatory for the process to start at all.
	aiServiceToken = "testinfra-ai-service-token"

	// What the stub model returns. confidence is deliberately far above
	// ghost.py's CONFIDENCE_CAP_WHEN_UNKNOWN so the capping rule is
	// observable in the result that comes back — see the assertion below.
	stubScore       = 82.5
	stubConfidence  = 0.95
	stubExplanation = "reposted four times with no candidate ever hired"

	// Token counts the stub reports. Arbitrary, but distinctive: seeing
	// exactly these numbers arrive in the Go Result proves usage is carried
	// from the gateway response through the Python capability onto the wire
	// and back into Go, rather than being defaulted anywhere along the way.
	stubPromptTokens     = 137
	stubCompletionTokens = 41
)

var stubTopSignals = []string{"repost_count", "always_hiring"}

// ghostGateway is an OpenAI-compatible chat-completions stub standing in for
// LiteLLM. apps/ai's gateway.py points langchain-openai at GATEWAY_URL with
// the capability's task key as the model name, so the requests recorded here
// are also the evidence that the ghost capability — and not some other one —
// ran, and that it asked for the task key it declares.
type ghostGateway struct {
	mu     sync.Mutex
	models []string
	// reply is the assistant content returned to every call. A test may
	// leave it as the valid ghost JSON or swap it to prove a failure path.
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

// The stub gateway and the AI service container are both process-wide
// singletons, started together on first use. They have to be: the container's
// GATEWAY_URL is baked in at start, and testcontainers tunnels exactly the one
// host port it was told about, so a second test that stood up its own listener
// would be handing the already-running container a port it cannot reach.
var (
	gatewayOnce sync.Once
	gatewayStub *ghostGateway
	gatewayErr  error
)

// startAIService brings up the stub gateway and the shipped apps/ai image,
// returning the stub so a test can inspect what the service asked the "model"
// for.
//
// Starting the container is itself an assertion: the wait strategy is
// /health/ready, which main.py answers only after pinging the broker, so this
// fails loudly if the image does not boot, its configuration does not
// validate, or its consumers never attach — rather than leaving the caller to
// time out later on a result that was never going to come.
func startAIService(t *testing.T) *ghostGateway {
	t.Helper()
	if err := startAIServiceErr(); err != nil {
		t.Fatal(err)
	}
	return gatewayStub
}

// startAIServiceErr is startAIService's error-returning form, for the one
// caller that runs off the test goroutine and therefore may not call t.Fatal.
func startAIServiceErr() error {
	gatewayOnce.Do(func() {
		// Exactly the JSON ghost.py's GhostResult accepts: it is
		// extra="forbid", so an added field here would fail structured-output
		// parsing three times over and turn into a `failed` result — itself a
		// real property, but not the one under test.
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
		// Deliberately not closed by a t.Cleanup: the container outlives the
		// test that happened to start it, and a closed listener would leave
		// every later test's model call refused.
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

		// The real script, from the repository, in the image compose pins —
		// so the account the service runs under is the account the stack
		// gives it, restrictions and all.
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

// ghostRequestedMessage mirrors the unexported struct of the same name in
// internal/ghostjob/application/publish.go: the envelope and the work payload
// embedded anonymously side by side, which is what produces the flat body
// consumers.py splits by field name. Declaring it the same way, out of the
// same two exported types, means a change to either type's json tags changes
// this test's wire bytes too — a hand-written JSON literal here would keep
// passing after Go's own shape drifted.
type ghostRequestedMessage struct {
	events.Envelope
	events.GhostWork
}

// newGhostRequest builds one ghost.requested body the way publish.go builds
// it, including the sha256 snapshot hash, and returns it with its envelope so
// assertions can compare the echoed correlation fields.
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
		// CrossBoardCount and AlwaysHiringCount deliberately left nil: that
		// is what makes ghost.py's has_unknown_signal true and lets the
		// confidence cap show up in the result.
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

// completedResult is one decoded ghost.completed delivery, as the Go result
// consumer handed it over.
type completedResult struct {
	envelope events.Envelope
	result   events.Result
}

// startResultConsumer runs the production result consumer — events.Consumer
// over events.ResultQueue, dispatching through ResultRegistry's
// HandleResultDelivery — and streams what it decodes onto a channel.
//
// Going through ResultRegistry rather than json.Unmarshal in the test is the
// point of this helper: HandleResultDelivery is where a real deployment
// decides a result is malformed, of an unknown event_type, or of an
// unimplemented schema_version and drops it. If Python's result body failed
// any of those gates, a bespoke decoder in the test would sail past it and
// the test would pass while production silently discarded every result.
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

// awaitResult waits for the next decoded ghost.completed, failing with a
// message that names what was being waited on rather than a bare timeout.
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

// TestGhostPython_Integration_ServiceWaitsForTopologyItCannotDeclare starts the
// AI service against a broker with no topology on it at all, and only then
// declares it from the Go side.
//
// This is the ordering compose does not guarantee. `ai` depends_on the broker
// being healthy and on rabbitmq-init having finished, neither of which says
// anything about whether the backend has run DeclareTopology yet — and since
// consumers.py declares nothing (M1-1) and is not permitted to, a cold stack
// routinely starts this service before there is anything for it to attach to.
// consumers.wait_for_topology exists for exactly that window: it passively
// asserts one of the work queues in a retry loop instead of connecting,
// failing and exiting.
//
// Which resource it probes is not incidental, and this test is the reason it
// is a queue. A passive declare is not permission-exempt — RabbitMQ still
// requires the account to hold configure, write or read on the resource — and
// `ai_service` holds none of the three on jobfinder.work, so probing the work
// exchange was refused permanently, indistinguishable to a retry loop from a
// backend that never starts. The work queues sit inside the read regex by
// design and are created by the same DeclareTopology call, so they answer the
// question the probe is actually asking.
//
// The assertion is the shape of the failure, not its absence. Without that
// wait, the service exits during the gap below and the container never becomes
// ready, so this test fails at startAIService with the container's own log
// attached. With it, the service sits there, converges the moment topology
// appears, and reports ready — which is what makes the deployment safe to
// start in any order.
//
// It must be the first test in this file: it is the only one that requires a
// broker nobody has declared topology on, every other test declares topology
// as its first act, and the broker (like the service container) is one per
// test binary. assertTopologyAbsent below fails loudly rather than quietly
// proving nothing if that ever stops being true.
func TestGhostPython_Integration_ServiceWaitsForTopologyItCannotDeclare(t *testing.T) {
	conn := dialTestBroker(t)
	assertTopologyAbsent(t, conn)

	started := make(chan error, 1)
	go func() {
		// startAIService is a t.Fatalf helper and must not run off the test's
		// own goroutine, so this mirrors it through the same sync.Once and
		// reports the error back instead.
		defer func() {
			if r := recover(); r != nil {
				started <- fmt.Errorf("starting the ai service panicked: %v", r)
			}
		}()
		started <- startAIServiceErr()
	}()

	// A deliberate gap with the service already dialling and no topology in
	// front of it. Long enough that a service which exits on a failed passive
	// declare has certainly done so (it fails on its first attempt, inside a
	// second), short enough to stay well inside wait_for_topology's own
	// 120s deadline.
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

	// Ready means main.py got past wait_for_topology, attached its consumers
	// and pinged the broker, all as an account that may not declare anything.
	// The round-trip test that follows is what proves those consumers work.
}

// topologyGap is how long the AI service is left running against an empty
// broker before topology appears.
const topologyGap = 8 * time.Second

// assertTopologyAbsent fails unless jobfinder.work does not exist yet, using a
// passive declaration — the same probe wait_for_topology makes from the Python
// side. A passive declare of a missing exchange is a channel-level NOT_FOUND,
// so this takes a throwaway channel.
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

// TestGhostPython_Integration_RealPythonServiceRoundTripsGhostWork drives one
// ghost.requested event through the real apps/ai container and asserts every
// hop of the round trip.
//
// The phases are sequential and share one container and one queue on purpose:
// starting the service is the expensive part, and RabbitMQ's per-work-type
// queues are a fixed, closed set (topology.go's WorkTypes), so two parallel
// tests over work.ghost would steal each other's deliveries.
func TestGhostPython_Integration_RealPythonServiceRoundTripsGhostWork(t *testing.T) {
	// Topology first, from Go, as the publisher's job (M1-1) — the same
	// ordering compose enforces with depends_on, and the only reason the
	// Python side finds anything to consume from.
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

	// --- Envelope correlation -------------------------------------------
	//
	// E1-3/M5-2/M5-3: the result echoes the request's correlation fields, so
	// the backend can tie a result to the work it asked for and to the
	// ledger entry admitting it. A service that minted fresh ids here would
	// leave every result unattributable, and idempotency.Admit would admit
	// duplicates forever.
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

	// --- Result payload --------------------------------------------------
	if got.result.Status != events.ResultSucceeded {
		t.Fatalf("result status = %q with failure %+v, want succeeded — the capability ran against a stub gateway that answers every call correctly", got.result.Status, got.result.Failure)
	}
	if got.result.SnapshotHash != request.SnapshotHash {
		t.Fatalf("result snapshot_hash = %q, want the request's %q — a result that cannot be tied back to the exact input it scored is not usable for cache invalidation", got.result.SnapshotHash, request.SnapshotHash)
	}

	// The capability-specific result is opaque to internal/events, so this
	// is where it stops being opaque: it must decode into the very type the
	// Go ghost path persists.
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
	// The confidence cap is the proof that the real capability ran rather
	// than something echoing the model back: ghost.py caps confidence at
	// CONFIDENCE_CAP_WHEN_UNKNOWN when any optional signal is unknown, and
	// the snapshot above leaves cross_board_count and always_hiring_count
	// unset. A transparent pass-through would have delivered 0.95.
	if scored.Confidence != 0.6 {
		t.Fatalf("confidence = %v, want 0.6 — ghost.py must cap confidence when the snapshot has unknown signals (the stub answered %v)", scored.Confidence, stubConfidence)
	}

	// --- Usage -----------------------------------------------------------
	if got.result.Usage == nil {
		t.Fatal("result carried no usage — token counts must survive the gateway response, the capability and the wire, or spend is invisible to the backend")
	}
	if got.result.Usage.InputTokens == nil || *got.result.Usage.InputTokens != stubPromptTokens {
		t.Fatalf("usage.input_tokens = %v, want %d", got.result.Usage.InputTokens, stubPromptTokens)
	}
	if got.result.Usage.OutputTokens == nil || *got.result.Usage.OutputTokens != stubCompletionTokens {
		t.Fatalf("usage.output_tokens = %v, want %d", got.result.Usage.OutputTokens, stubCompletionTokens)
	}

	// --- Gateway routing --------------------------------------------------
	//
	// FR-010a: a capability reaches a model only through the gateway, keyed
	// by its task key. The stub saw the model name the service asked for, so
	// this catches a capability that started naming a concrete provider
	// model (which would bypass gateway/config.yaml's routing entirely).
	models := gateway.seen()
	if len(models) == 0 {
		t.Fatal("the stub gateway was never called — the result was produced without a model call at all")
	}
	for _, model := range models {
		if model != "ghost" {
			t.Fatalf("the service asked the gateway for model %q, want the task key \"ghost\"", model)
		}
	}

	// --- The work queue drained -------------------------------------------
	//
	// M3-2: the delivery is acked only after the result publish is confirmed.
	// A queue still holding the message means the ack never happened and the
	// same work will be redelivered forever; an unacked message means the
	// consumer is wedged holding it.
	assertQueueDrained(t, conn, "work.ghost", "after the Python service completed the work")
}

// TestGhostPython_Integration_MalformedWorkDoesNotWedgeTheConsumer publishes a
// body the Python consumer cannot possibly parse and then publishes a valid
// one, asserting the valid one still completes.
//
// A parse failure must be terminal for that one message and invisible to
// every other: if it instead requeued forever, one poisoned publish (a
// half-rolled-out schema change, a hand-crafted replay) would occupy the
// consumer and starve all real ghost work — the classic poison-message stall,
// and precisely the failure the DLX arguments on work.ghost exist to prevent.
//
// It runs after the round-trip test in file order and reuses that test's
// container, so it inherits the same fixed queue and must not run in parallel
// with it.
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

	// Valid JSON, valid envelope shape, and a payload the generated GhostWork
	// model rejects outright (`snapshot` missing, `extra="forbid"` on the
	// junk field). This is the realistic shape of a schema drift, not a
	// random byte string.
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

	// The unparseable message must be gone from work.ghost, not sitting
	// there ready for another doomed attempt or held unacked by a stuck
	// consumer.
	assertQueueDrained(t, conn, "work.ghost", "after an unparseable message was rejected")

	// And gone to the right place: work.ghost's x-dead-letter-exchange
	// arguments (topology.go) exist so a message the consumer cannot handle
	// is kept for inspection instead of vanishing. This is the assertion
	// that pins FastStream's rejection *policy*, not just its effect — its
	// default AckPolicy rejects without requeue, which is what routes the
	// delivery through the DLX. A future FastStream default of
	// requeue-on-error would leave dlq.ghost empty and work.ghost spinning.
	assertQueueDepth(t, conn, "dlq.ghost", 1, "the unparseable message must be dead-lettered, not discarded")
}

// assertQueueDrained polls a queue until it reports no messages, so a test
// does not race the broker's own accounting (depth is updated asynchronously
// after an ack). It fails naming the queue and the depth it was stuck at.
func assertQueueDrained(t *testing.T, conn *amqp.Connection, queueName, why string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	var last int
	for time.Now().Before(deadline) {
		ch, err := conn.Channel()
		if err != nil {
			t.Fatalf("open channel to inspect %s: %v", queueName, err)
		}
		// QueueDeclarePassive, not the deprecated QueueInspect: passive
		// declare asserts the queue and reports its depth without needing
		// configure rights, which is also all this connection has.
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

// assertQueueDepth polls until a queue reports exactly want messages, giving
// the broker's asynchronous accounting time to settle before failing.
func assertQueueDepth(t *testing.T, conn *amqp.Connection, queueName string, want int, why string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	var last int
	for time.Now().Before(deadline) {
		ch, err := conn.Channel()
		if err != nil {
			t.Fatalf("open channel to inspect %s: %v", queueName, err)
		}
		// QueueDeclarePassive, not the deprecated QueueInspect: passive
		// declare asserts the queue and reports its depth without needing
		// configure rights, which is also all this connection has.
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
