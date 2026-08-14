# Build + run the Go API. Context = repo root (apps/api is its own Go
# module; no other workspace files are needed at build time).
FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download

COPY apps/api .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------------------------------------------------------------------------
# Runtime: static Go binary + chromium (chromedp PDF rendering) + poppler
# (pdftotext fallback for resume PDF import). RenderCV rendering is in-process
# via rendercv-go (embedded WASI Typst compiler) — no Python or external
# RenderCV binary needed.
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
      chromium \
      poppler-utils \
      ca-certificates \
      fonts-liberation \
      curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/server /usr/local/bin/server

ENV DOCUMENTS_DIR=/data/documents
# chromedp auto-detects a system Chromium via PATH lookup (tries
# "chromium", "chromium-browser", "google-chrome", ...) — the Debian
# `chromium` package installs it under that exact name, no extra flag needed.

EXPOSE 3000
# Goose migrations run automatically on startup (internal/db.Migrate),
# unlike the NestJS image's separate `drizzle-kit migrate` CMD step.
CMD ["/usr/local/bin/server"]
