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
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
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
	minioImage        = "minio/minio:latest"
	redisImage        = "redis:7-alpine"
	flareSolverrImage = "ghcr.io/flaresolverr/flaresolverr:latest"
	chromeImage       = "chromedp/headless-shell:latest"
	// Pinned to what docker-compose.yml runs, so the proxy that answers in a
	// test is the proxy that answers in the dev stack.
	liteLLMImage = "ghcr.io/berriai/litellm:main-stable"
)

// Credentials the containers are created with. They are fixed rather than
// random because they appear in DSNs in test failure output, and a container
// that lives for one test process has nothing to protect.
const (
	dbName   = "jobfinder_test"
	dbUser   = "jobfinder"
	dbPass   = "jobfinder"
	mqUser   = "jobfinder"
	mqPass   = "jobfinder"
	chDB     = "default"
	chUser   = "default"
	chPass   = "clickhouse"
	startTTL = 3 * time.Minute
)

// LiteLLMMasterKey is the master key the proxy container is created with.
// Callers must send it as `Authorization: Bearer <key>`; the proxy rejects an
// unauthenticated request the same way the deployed one does.
const LiteLLMMasterKey = "sk-testinfra-master-key"

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

	rabbitOnce   sync.Once
	rabbitResult result

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

		container, err := rabbitmq.Run(ctx, rabbitMQImage,
			rabbitmq.WithAdminUsername(mqUser),
			rabbitmq.WithAdminPassword(mqPass),
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
			// The image listens on loopback by default, which is
			// unreachable from outside the container.
			testcontainers.WithCmdArgs(
				"--remote-debugging-address=0.0.0.0",
				"--remote-debugging-port=9222",
				"--no-sandbox",
				"--disable-gpu",
				"--headless=new",
			),
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
