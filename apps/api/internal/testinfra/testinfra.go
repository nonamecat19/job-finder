//go:build integration

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

const (
	postgresImage = "pgvector/pgvector:pg16"

	rabbitMQImage     = "rabbitmq:4.3.4-management-alpine"
	clickHouseImage   = "clickhouse/clickhouse-server:26.4.5"
	rabbitInitImage   = "curlimages/curl:8.11.1"
	minioImage        = "minio/minio:latest"
	redisImage        = "redis:7-alpine"
	flareSolverrImage = "ghcr.io/flaresolverr/flaresolverr:latest"
	chromeImage       = "chromedp/headless-shell:latest"

	liteLLMImage = "ghcr.io/berriai/litellm:main-stable"
)

const (
	aiImageRepo = "jobfinder-ai"
	aiImageTag  = "testinfra"

	aiBuildTTL = 15 * time.Minute
)

const (
	dbName   = "jobfinder_test"
	dbUser   = "jobfinder"
	dbPass   = "jobfinder"
	chDB     = "default"
	chUser   = "default"
	chPass   = "clickhouse"
	startTTL = 3 * time.Minute
)

const LiteLLMMasterKey = "sk-testinfra-master-key"

const (
	MQAdminUser = "jobfinder"
	MQAdminPass = "jobfinder"
)

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

func PostgresDSN(ctx context.Context) (string, error) {
	postgresOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		container, err := postgres.Run(ctx, postgresImage,
			postgres.WithDatabase(dbName),
			postgres.WithUsername(dbUser),
			postgres.WithPassword(dbPass),

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

func RabbitMQURL(ctx context.Context) (string, error) {
	rabbitOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

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

func ClickHouseDSN(ctx context.Context) (string, error) {
	clickHouseOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		container, err := clickhouse.Run(ctx, clickHouseImage,
			clickhouse.WithDatabase(chDB),
			clickhouse.WithUsername(chUser),
			clickhouse.WithPassword(chPass),

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

func LiteLLM(ctx context.Context, upstreamPort int) (string, error) {
	liteLLMOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		configPath, err := repoFile("gateway", "config.yaml")
		if err != nil {
			liteLLMResult.err = err
			return
		}

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

func repoFile(parts ...string) (string, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("testinfra: cannot locate own source file")
	}

	root := filepath.Join(filepath.Dir(self), "..", "..", "..", "..")
	path := filepath.Join(append([]string{root}, parts...)...)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("testinfra: %s: %w", filepath.Join(parts...), err)
	}
	return path, nil
}

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

func ChromeWebSocketURL(ctx context.Context, upstreamPort int) (string, error) {
	chromeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTTL)
		defer cancel()

		container, err := testcontainers.Run(ctx, chromeImage,
			testcontainers.WithHostPortAccess(upstreamPort),

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

		chromeResult.dsn = fmt.Sprintf("ws://%s", net.JoinHostPort(host, port))
	})
	return chromeResult.dsn, chromeResult.err
}

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

type AIServiceConfig struct {
	BrokerUser string
	BrokerPass string

	GatewayPort int

	ServiceToken string
}

func AIService(ctx context.Context, cfg AIServiceConfig) (string, error) {
	aiOnce.Do(func() {

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

func detachRegistryCredentials() {
	if _, set := os.LookupEnv("DOCKER_AUTH_CONFIG"); !set {
		_ = os.Setenv("DOCKER_AUTH_CONFIG", "{}")
	}
}
