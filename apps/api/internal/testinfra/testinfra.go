//go:build integration

// Package testinfra starts the real backing services the integration suite
// runs against — Postgres, RabbitMQ, ClickHouse and the LiteLLM proxy — as
// throwaway Docker containers via testcontainers.
//
// Every integration test gets its infrastructure from here rather than from
// an externally provisioned service named by an environment variable. That
// makes the suite hermetic: `go test -tags integration ./...` needs a Docker
// daemon and nothing else, no test can observe data left behind by a previous
// run or by a developer's dev stack, and no test is silently skipped because
// a service happened not to be running.
//
// One container of each kind is started per test binary, lazily, on first
// use, and shared by every test in that package. Nothing terminates them
// explicitly: testcontainers' reaper (ryuk) removes them when the test
// process exits, including when it panics or is killed.
package testinfra

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Images mirror docker-compose.yml exactly, so the service a test runs
// against is the service the stack runs. Where compose floats on :latest
// (MinIO, FlareSolverr) this does too, deliberately: a test that pinned
// tighter than the stack would stop being evidence about the stack.
const (
	// pgvector, not stock postgres: the schema declares vector(768) columns
	// (internal/db/migrations), which stock postgres cannot even create.
	postgresImage = "pgvector/pgvector:pg16"
	// The management plugin is not optional here: docker/rabbitmq/
	// init-ai-user.sh provisions the AI service account over the management
	// HTTP API, and internal/events tests purge queues the same way.
	rabbitMQImage     = "rabbitmq:4.3.4-management-alpine"
	clickHouseImage   = "clickhouse/clickhouse-server:26.4.5"
	rabbitInitImage   = "curlimages/curl:8.11.1"
	minioImage        = "minio/minio:latest"
	redisImage        = "redis:7-alpine"
	flareSolverrImage = "ghcr.io/flaresolverr/flaresolverr:latest"
	chromeImage       = "chromedp/headless-shell:latest"
	// Pinned to what docker-compose.yml runs, so the proxy that answers in a
	// test is the proxy that answers in the dev stack.
	liteLLMImage = "ghcr.io/berriai/litellm:main-stable"
)

// The AI orchestration service is not pulled but built, from apps/ai/
// Dockerfile in this working tree, so a test always runs the code in front of
// it. The tag is fixed rather than random so successive runs (and successive
// test binaries) reuse the same built image instead of producing a new
// dangling one each time.
const (
	aiImageRepo = "jobfinder-ai"
	aiImageTag  = "testinfra"
	// A cold `uv sync` of the whole dependency set (langchain, langgraph,
	// faststream) is minutes of work; startTTL is sized for pulling a
	// prebuilt image and is far too tight for that first build.
	aiBuildTTL = 15 * time.Minute
)

// Credentials the containers are created with. They are fixed rather than
// random because they appear in DSNs in test failure output, and a container
// that lives for one test process has nothing to protect.
const (
	dbName   = "jobfinder_test"
	dbUser   = "jobfinder"
	dbPass   = "jobfinder"
	chDB     = "default"
	chUser   = "default"
	chPass   = "clickhouse"
	startTTL = 3 * time.Minute
)

// LiteLLMMasterKey is the master key the proxy container is created with.
// Callers must send it as `Authorization: Bearer <key>`; the proxy rejects an
// unauthenticated request the same way the deployed one does.
const LiteLLMMasterKey = "sk-testinfra-master-key"

// The broker's administrator account — the RABBITMQ_DEFAULT_USER of
// docker-compose.yml, which owns topology and is what internal/events
// connects as.
const (
	MQAdminUser = "jobfinder"
	MQAdminPass = "jobfinder"
)

// MinIO credentials and bucket, matching docker-compose.yml's defaults so a
// test exercises the same names the stack does.
const (
	MinIOAccessKey = "minioadmin"
	MinIOSecretKey = "minioadmin"
	MinIOBucket    = "documents"
)

type result struct {
	dsn string
	err error
}

var (
	postgresOnce   sync.Once
	postgresResult result

	rabbitOnce    sync.Once
	rabbitResult  result
	rabbitNetwork *testcontainers.DockerNetwork

	clickHouseOnce   sync.Once
	clickHouseResult result

	liteLLMOnce   sync.Once
	liteLLMResult result

	minioOnce   sync.Once
	minioResult result

	redisOnce   sync.Once
	redisResult result

	flareSolverrOnce   sync.Once
	flareSolverrResult result

	chromeOnce   sync.Once
	chromeResult result

	aiOnce   sync.Once
	aiResult result
)

// PostgresDSN starts (once per process) a pgvector-enabled Postgres and
// returns a DSN for its maintenance database. Callers that need an isolated
// schema should go through internal/dbtest, which clones a migrated template
// database per suite on top of this server.
func PostgresDSN(ctx context.Context) (string, error) {
	postgresOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		container, err := postgres.Run(ctx, postgresImage,
			postgres.WithDatabase(dbName),
			postgres.WithUsername(dbUser),
			postgres.WithPassword(dbPass),
			// The default fsync=on costs more than it buys for a database
			// that is deleted when the process exits.
			testcontainers.WithCmdArgs("-c", "fsync=off", "-c", "full_page_writes=off", "-c", "max_connections=200"),
			postgres.BasicWaitStrategies(),
		)
		if err != nil {
			postgresResult.err = fmt.Errorf("testinfra: start postgres: %w", err)
			return
		}
		dsn, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			postgresResult.err = fmt.Errorf("testinfra: postgres dsn: %w", err)
			return
		}
		postgresResult.dsn = dsn
	})
	return postgresResult.dsn, postgresResult.err
}

// RabbitMQURL starts (once per process) a RabbitMQ broker and returns its
// amqp:// URL. The broker is empty: every exchange, queue and binding the
// tests use is declared by the code under test.
func RabbitMQURL(ctx context.Context) (string, error) {
	rabbitOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		// A dedicated network with the `rabbitmq` alias, because
		// docker/rabbitmq/init-ai-user.sh addresses the broker by that
		// hostname (it is a compose service name) and this package runs that
		// script verbatim rather than a paraphrase of it.
		nw, err := network.New(ctx)
		if err != nil {
			rabbitResult.err = fmt.Errorf("testinfra: create network: %w", err)
			return
		}
		rabbitNetwork = nw

		container, err := rabbitmq.Run(ctx, rabbitMQImage,
			rabbitmq.WithAdminUsername(MQAdminUser),
			rabbitmq.WithAdminPassword(MQAdminPass),
			network.WithNetwork([]string{"rabbitmq"}, nw),
		)
		if err != nil {
			rabbitResult.err = fmt.Errorf("testinfra: start rabbitmq: %w", err)
			return
		}
		url, err := container.AmqpURL(ctx)
		if err != nil {
			rabbitResult.err = fmt.Errorf("testinfra: rabbitmq url: %w", err)
			return
		}
		rabbitResult.dsn = url
	})
	return rabbitResult.dsn, rabbitResult.err
}

// RabbitMQURLAs returns an amqp:// URL for the running broker under another
// account, so a test can act as the restricted AI service user rather than as
// the administrator RabbitMQURL hands out.
func RabbitMQURLAs(ctx context.Context, user, password string) (string, error) {
	admin, err := RabbitMQURL(ctx)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(admin)
	if err != nil {
		return "", fmt.Errorf("testinfra: parse broker url: %w", err)
	}
	u.User = url.UserPassword(user, password)
	return u.String(), nil
}

// ProvisionRabbitMQAIUser runs docker/rabbitmq/init-ai-user.sh — the actual
// script the `rabbitmq-init` compose service runs, from the repository, in
// the image compose pins — against the running broker.
//
// Running the file rather than reimplementing its two API calls is the point:
// what needs proving is that the script provisions an account with exactly
// the permissions contracts/messaging.md M7-3 describes, and a paraphrase of
// it here would prove that only about the paraphrase.
func ProvisionRabbitMQAIUser(ctx context.Context, aiUser, aiPass string) error {
	if _, err := RabbitMQURL(ctx); err != nil {
		return err
	}

	script, err := repoFile("docker", "rabbitmq", "init-ai-user.sh")
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
	defer cancel()

	container, err := testcontainers.Run(ctx, rabbitInitImage,
		testcontainers.WithEntrypointArgs("/bin/sh", "/init-ai-user.sh"),
		network.WithNetwork([]string{"rabbitmq-init"}, rabbitNetwork),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			HostFilePath:      script,
			ContainerFilePath: "/init-ai-user.sh",
			FileMode:          0o555,
		}),
		testcontainers.WithEnv(map[string]string{
			"RABBITMQ_DEFAULT_USER": MQAdminUser,
			"RABBITMQ_DEFAULT_PASS": MQAdminPass,
			"RABBITMQ_AI_USER":      aiUser,
			"RABBITMQ_AI_PASS":      aiPass,
		}),
		testcontainers.WithWaitStrategy(
			wait.ForExit().WithExitTimeout(startTTL),
		),
	)
	if err != nil {
		return fmt.Errorf("testinfra: run init-ai-user.sh: %w", err)
	}

	state, err := container.State(ctx)
	if err != nil {
		return fmt.Errorf("testinfra: init-ai-user.sh state: %w", err)
	}
	if state.ExitCode != 0 {
		logs, _ := container.Logs(ctx)
		var output []byte
		if logs != nil {
			output, _ = io.ReadAll(logs)
			logs.Close()
		}
		return fmt.Errorf("testinfra: init-ai-user.sh exited %d: %s", state.ExitCode, output)
	}
	return nil
}

// ClickHouseDSN starts (once per process) a ClickHouse server and returns a
// native-protocol DSN for it. The server holds no tables: the retention tests
// create Langfuse's real DDL themselves.
func ClickHouseDSN(ctx context.Context) (string, error) {
	clickHouseOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		container, err := clickhouse.Run(ctx, clickHouseImage,
			clickhouse.WithDatabase(chDB),
			clickhouse.WithUsername(chUser),
			clickhouse.WithPassword(chPass),
			// Port-listening alone is not readiness: ClickHouse binds 9000
			// before it can complete a handshake, and a test that dials that
			// early gets "connection reset by peer". /ping over HTTP answers
			// only once the server really is serving.
			testcontainers.WithWaitStrategy(
				wait.ForHTTP("/ping").
					WithPort("8123/tcp").
					WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
					WithStartupTimeout(startTTL),
			),
		)
		if err != nil {
			clickHouseResult.err = fmt.Errorf("testinfra: start clickhouse: %w", err)
			return
		}
		host, err := container.Host(ctx)
		if err != nil {
			clickHouseResult.err = fmt.Errorf("testinfra: clickhouse host: %w", err)
			return
		}
		port, err := container.MappedPort(ctx, "9000/tcp")
		if err != nil {
			clickHouseResult.err = fmt.Errorf("testinfra: clickhouse port: %w", err)
			return
		}
		clickHouseResult.dsn = fmt.Sprintf("clickhouse://%s:%s@%s:%s/%s", chUser, chPass, host, port.Port(), chDB)
	})
	return clickHouseResult.dsn, clickHouseResult.err
}

// LiteLLM starts (once per process) the LiteLLM proxy on the repository's
// real gateway/config.yaml — the same file docker-compose.yml mounts — and
// returns its base URL.
//
// The point is the config: nothing else proves that what LiteLLM accepts and
// what gateway/config.yaml says agree, because the Go build never reads that
// file and the guardrails in internal/platform/llm only parse it as YAML. A
// config the proxy rejects never becomes ready, so the wait strategy below is
// itself an assertion.
//
// upstreamPort is a port on the host serving an OpenAI-compatible stub. Both
// provider base URLs are pointed at it, so every tier in every chain resolves
// to that stub and no request reaches OpenRouter, OpenAI, or any other
// provider: the proxy does real routing, retries and fallbacks over fake
// models. The API keys are placeholders for the same reason — LiteLLM
// resolves os.environ/* at config load, so an absent variable would fail the
// load rather than a request.
func LiteLLM(ctx context.Context, upstreamPort int) (string, error) {
	liteLLMOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		configPath, err := repoFile("gateway", "config.yaml")
		if err != nil {
			liteLLMResult.err = err
			return
		}

		// testcontainers publishes a host port to the container under this
		// name (an SSH tunnel sidecar); the proxy dials it as if it were a
		// provider's public API.
		upstream := fmt.Sprintf("http://host.testcontainers.internal:%d/v1", upstreamPort)

		container, err := testcontainers.Run(ctx, liteLLMImage,
			testcontainers.WithCmdArgs("--port", "4000", "--config", "/app/config.yaml"),
			testcontainers.WithHostPortAccess(upstreamPort),
			testcontainers.WithFiles(testcontainers.ContainerFile{
				HostFilePath:      configPath,
				ContainerFilePath: "/app/config.yaml",
				FileMode:          0o444,
			}),
			testcontainers.WithEnv(map[string]string{
				"LITELLM_MASTER_KEY":  LiteLLMMasterKey,
				"OPENROUTER_API_KEY":  "sk-testinfra-not-a-real-key",
				"OPENAI_API_KEY":      "sk-testinfra-not-a-real-key",
				"OPENROUTER_API_BASE": upstream,
				"OPENAI_API_BASE":     upstream,
				// Empty is what makes the Langfuse callbacks inert, exactly
				// as in a dev stack with no collector configured.
				"LANGFUSE_PUBLIC_KEY": "",
				"LANGFUSE_SECRET_KEY": "",
				"LANGFUSE_HOST":       "",
			}),
			testcontainers.WithExposedPorts("4000/tcp"),
			testcontainers.WithWaitStrategy(
				wait.ForHTTP("/health/liveliness").
					WithPort("4000/tcp").
					WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
					WithStartupTimeout(startTTL),
			),
		)
		if err != nil {
			liteLLMResult.err = fmt.Errorf("testinfra: start litellm on gateway/config.yaml: %w", err)
			return
		}
		host, err := container.Host(ctx)
		if err != nil {
			liteLLMResult.err = fmt.Errorf("testinfra: litellm host: %w", err)
			return
		}
		port, err := container.MappedPort(ctx, "4000/tcp")
		if err != nil {
			liteLLMResult.err = fmt.Errorf("testinfra: litellm port: %w", err)
			return
		}
		liteLLMResult.dsn = fmt.Sprintf("http://%s:%s", host, port.Port())
	})
	return liteLLMResult.dsn, liteLLMResult.err
}

// repoFile resolves a path relative to the repository root from this source
// file's own location, so it holds wherever `go test` is invoked from.
func repoFile(parts ...string) (string, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("testinfra: cannot locate own source file")
	}
	// <root>/apps/api/internal/testinfra/testinfra.go
	root := filepath.Join(filepath.Dir(self), "..", "..", "..", "..")
	path := filepath.Join(append([]string{root}, parts...)...)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("testinfra: %s: %w", filepath.Join(parts...), err)
	}
	return path, nil
}

// MinIOEndpoint starts (once per process) a MinIO server and returns its
// host:port, which is the form minio.Config.Endpoint takes. The server has no
// buckets: creating MinIOBucket is the adapter's own job (Store.ensureBucket),
// and a test that pre-made it would hide that.
func MinIOEndpoint(ctx context.Context) (string, error) {
	minioOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		container, err := testcontainers.Run(ctx, minioImage,
			testcontainers.WithCmdArgs("server", "/data"),
			testcontainers.WithEnv(map[string]string{
				"MINIO_ROOT_USER":     MinIOAccessKey,
				"MINIO_ROOT_PASSWORD": MinIOSecretKey,
			}),
			testcontainers.WithExposedPorts("9000/tcp"),
			testcontainers.WithWaitStrategy(
				wait.ForHTTP("/minio/health/live").
					WithPort("9000/tcp").
					WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
					WithStartupTimeout(startTTL),
			),
		)
		if err != nil {
			minioResult.err = fmt.Errorf("testinfra: start minio: %w", err)
			return
		}
		host, port, err := hostPort(ctx, container, "9000/tcp")
		if err != nil {
			minioResult.err = fmt.Errorf("testinfra: minio endpoint: %w", err)
			return
		}
		minioResult.dsn = net.JoinHostPort(host, port)
	})
	return minioResult.dsn, minioResult.err
}

// RedisURL starts (once per process) a Redis server and returns a redis:// URL
// for database 0. Callers that care about the database index should rewrite
// the path — that parsing is exactly what internal/queue's tests exercise.
func RedisURL(ctx context.Context) (string, error) {
	redisOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		container, err := testcontainers.Run(ctx, redisImage,
			testcontainers.WithExposedPorts("6379/tcp"),
			testcontainers.WithWaitStrategy(
				wait.ForListeningPort("6379/tcp").WithStartupTimeout(startTTL),
			),
		)
		if err != nil {
			redisResult.err = fmt.Errorf("testinfra: start redis: %w", err)
			return
		}
		host, port, err := hostPort(ctx, container, "6379/tcp")
		if err != nil {
			redisResult.err = fmt.Errorf("testinfra: redis endpoint: %w", err)
			return
		}
		redisResult.dsn = fmt.Sprintf("redis://%s", net.JoinHostPort(host, port))
	})
	return redisResult.dsn, redisResult.err
}

// FlareSolverrURL starts (once per process) FlareSolverr and returns its base
// URL, in the form the config's FLARESOLVERR_URL takes.
//
// upstreamPort is a host port the container must be able to reach, so a test
// can point FlareSolverr at a local site instead of the public internet.
// The image bundles a browser, so this is the slowest container here.
func FlareSolverrURL(ctx context.Context, upstreamPort int) (string, error) {
	flareSolverrOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		container, err := testcontainers.Run(ctx, flareSolverrImage,
			testcontainers.WithHostPortAccess(upstreamPort),
			testcontainers.WithEnv(map[string]string{"LOG_LEVEL": "warning"}),
			testcontainers.WithExposedPorts("8191/tcp"),
			testcontainers.WithWaitStrategy(
				wait.ForHTTP("/").
					WithPort("8191/tcp").
					WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
					WithStartupTimeout(startTTL),
			),
		)
		if err != nil {
			flareSolverrResult.err = fmt.Errorf("testinfra: start flaresolverr: %w", err)
			return
		}
		host, port, err := hostPort(ctx, container, "8191/tcp")
		if err != nil {
			flareSolverrResult.err = fmt.Errorf("testinfra: flaresolverr endpoint: %w", err)
			return
		}
		flareSolverrResult.dsn = fmt.Sprintf("http://%s", net.JoinHostPort(host, port))
	})
	return flareSolverrResult.dsn, flareSolverrResult.err
}

// ChromeWebSocketURL starts (once per process) a headless Chrome and returns
// its DevTools websocket URL, which chromedp.NewRemoteAllocator takes.
//
// It exists because the browser paths (internal/scraping, the PDF renderer)
// otherwise need a Chrome binary installed on whatever machine runs the
// tests, which a CI runner does not have — so those paths had no test at all.
//
// upstreamPort is a host port the browser must be able to reach, so a test
// can serve it a page without going to the internet.
func ChromeWebSocketURL(ctx context.Context, upstreamPort int) (string, error) {
	chromeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		container, err := testcontainers.Run(ctx, chromeImage,
			testcontainers.WithHostPortAccess(upstreamPort),
			// No command override: the image's own run.sh already binds
			// the DevTools endpoint to 0.0.0.0:9222 with the sandbox
			// flags a container needs, and replacing it with a bare
			// browser invocation is what makes the endpoint never come up.
			testcontainers.WithExposedPorts("9222/tcp"),
			testcontainers.WithWaitStrategy(
				wait.ForHTTP("/json/version").
					WithPort("9222/tcp").
					WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
					WithStartupTimeout(startTTL),
			),
		)
		if err != nil {
			chromeResult.err = fmt.Errorf("testinfra: start chrome: %w", err)
			return
		}
		host, port, err := hostPort(ctx, container, "9222/tcp")
		if err != nil {
			chromeResult.err = fmt.Errorf("testinfra: chrome endpoint: %w", err)
			return
		}
		// chromedp dials the browser endpoint directly rather than the
		// per-target one, so the plain host:port form is what it wants.
		chromeResult.dsn = fmt.Sprintf("ws://%s", net.JoinHostPort(host, port))
	})
	return chromeResult.dsn, chromeResult.err
}

// hostPort resolves a container's reachable host and mapped port for one of
// its exposed ports.
func hostPort(ctx context.Context, container testcontainers.Container, port string) (string, string, error) {
	host, err := container.Host(ctx)
	if err != nil {
		return "", "", err
	}
	mapped, err := container.MappedPort(ctx, port)
	if err != nil {
		return "", "", err
	}
	return host, mapped.Port(), nil
}

// AIServiceConfig configures the apps/ai container AIService starts.
type AIServiceConfig struct {
	// BrokerUser/BrokerPass are the broker account the service connects as.
	// A test provisions them with ProvisionRabbitMQAIUser first, so the
	// service runs under exactly the restricted account compose gives it
	// rather than as the administrator.
	BrokerUser string
	BrokerPass string
	// GatewayPort is a port on the host serving an OpenAI-compatible stub.
	// GATEWAY_URL points at it, so every model call the service makes lands
	// on the test's own handler and no provider is ever contacted.
	GatewayPort int
	// ServiceToken is AI_SERVICE_TOKEN — required for the process to boot at
	// all (settings.REQUIRED_ENV_KEYS), and the shared secret its HTTP
	// surface authenticates with.
	ServiceToken string
}

// AIService builds apps/ai's image from this repository and runs it against
// the RabbitMQ container this package owns, returning the service's base URL
// on the host.
//
// The image is built the way docker-compose.yml's `ai` service builds it —
// context = the repository root, dockerfile = apps/ai/Dockerfile — so what
// runs here is the artifact that ships, not a paraphrase of it assembled by
// the test. The build is expensive the first time and nearly free afterwards:
// Docker's layer cache is shared with compose's, and KeepImage stops the
// reaper from deleting the result when the test binary exits.
//
// The container joins the same network as the broker (see rabbitNetwork) and
// addresses it as `rabbitmq:5672`, which is what compose's RABBITMQ_URL does
// and therefore what the AMQP permissions the init script writes are scoped
// against.
//
// Readiness is /health/ready, not /health/live: live only proves the process
// started, while ready additionally pings the broker (main.py), so a service
// that booted but could not reach or authenticate against RabbitMQ never
// becomes ready and the caller fails with a startup error instead of a
// mystifying "no result ever arrived" timeout minutes later.
func AIService(ctx context.Context, cfg AIServiceConfig) (string, error) {
	aiOnce.Do(func() {
		// The broker must exist before the service is told to dial it, and
		// starting it here also populates rabbitNetwork.
		if _, err := RabbitMQURL(ctx); err != nil {
			aiResult.err = err
			return
		}

		root, err := repoRoot()
		if err != nil {
			aiResult.err = err
			return
		}
		detachRegistryCredentials()

		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), aiBuildTTL)
		defer cancel()

		gateway := fmt.Sprintf("http://host.testcontainers.internal:%d", cfg.GatewayPort)
		brokerURL := fmt.Sprintf("amqp://%s:%s@rabbitmq:5672/", cfg.BrokerUser, cfg.BrokerPass)

		container, err := testcontainers.Run(ctx, "",
			testcontainers.WithDockerfile(testcontainers.FromDockerfile{
				Context:    root,
				Dockerfile: "apps/ai/Dockerfile",
				Repo:       aiImageRepo,
				Tag:        aiImageTag,
				KeepImage:  true,
			}),
			network.WithNetwork([]string{"ai"}, rabbitNetwork),
			testcontainers.WithHostPortAccess(cfg.GatewayPort),
			testcontainers.WithEnv(map[string]string{
				"GATEWAY_URL":        gateway,
				"LITELLM_MASTER_KEY": LiteLLMMasterKey,
				"RABBITMQ_URL":       brokerURL,
				"AI_SERVICE_TOKEN":   cfg.ServiceToken,
				// Empty is what makes Langfuse inert, exactly as in a dev
				// stack with no collector configured: the client disables
				// itself rather than exporting to cloud.langfuse.com.
				"LANGFUSE_PUBLIC_KEY": "",
				"LANGFUSE_SECRET_KEY": "",
				"LANGFUSE_HOST":       "",
				"LOG_LEVEL":           "INFO",
			}),
			testcontainers.WithExposedPorts("8000/tcp"),
			testcontainers.WithWaitStrategy(
				wait.ForHTTP("/health/ready").
					WithPort("8000/tcp").
					WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
					WithStartupTimeout(startTTL),
			),
		)
		if err != nil {
			aiResult.err = fmt.Errorf("testinfra: build+start apps/ai: %w%s", err, aiLogs(ctx, container))
			return
		}
		host, port, err := hostPort(ctx, container, "8000/tcp")
		if err != nil {
			aiResult.err = fmt.Errorf("testinfra: ai service endpoint: %w", err)
			return
		}
		aiResult.dsn = fmt.Sprintf("http://%s", net.JoinHostPort(host, port))
	})
	return aiResult.dsn, aiResult.err
}

// aiLogs appends the container's own stdout/stderr to a startup failure.
// Without it a service that boots and then dies — a refused AMQP
// declaration, a missing environment variable — surfaces only as a wait
// strategy timeout, which names nothing about the actual cause.
func aiLogs(ctx context.Context, container *testcontainers.DockerContainer) string {
	if container == nil {
		return ""
	}
	reader, err := container.Logs(ctx)
	if err != nil {
		return ""
	}
	defer reader.Close()
	output, err := io.ReadAll(reader)
	if err != nil || len(output) == 0 {
		return ""
	}
	return "\n--- apps/ai container logs ---\n" + string(output)
}

// repoRoot resolves the repository root the same way repoFile does, for
// callers that need the directory itself (a Docker build context).
func repoRoot() (string, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("testinfra: cannot locate own source file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(self), "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "apps", "ai", "Dockerfile")); err != nil {
		return "", fmt.Errorf("testinfra: repository root: %w", err)
	}
	return root, nil
}

// detachRegistryCredentials makes the image build ignore whatever registry
// credentials the developer's ~/.docker/config.json points at.
//
// Building from a Dockerfile is the one thing testcontainers does that reads
// that file: it resolves an auth config for every FROM image, and it treats a
// failing credential helper as fatal, where the docker CLI only warns. A
// machine with a stale credHelpers entry (an expired cloud login, a deleted
// account) therefore cannot build this image at all, with an error naming
// registries this repository does not use — python:3.13-slim comes from Docker
// Hub anonymously.
//
// Every image the suite touches is public, so having no credentials is the
// correct configuration here, and an explicitly empty one is also more
// hermetic than reading the developer's. It is left overridable: a caller that
// really does need authenticated pulls can set DOCKER_AUTH_CONFIG itself.
// detachRegistryCredentials stops testcontainers from consulting the
// machine's docker credential helpers while building an image.
//
// Every image this package pulls is public, but testcontainers resolves an
// auth config for each FROM at build time and treats a failing credential
// helper as fatal, where the docker CLI only warns. A stale gcloud helper on
// a developer's machine therefore makes the build impossible with an error
// naming registries this repository has never used. An explicitly empty
// config is both correct here and more hermetic — and it only applies when
// DOCKER_AUTH_CONFIG is unset, so a CI runner that does need registry
// credentials keeps them.
func detachRegistryCredentials() {
	if _, set := os.LookupEnv("DOCKER_AUTH_CONFIG"); !set {
		_ = os.Setenv("DOCKER_AUTH_CONFIG", "{}")
	}
}
