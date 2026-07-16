# Build + run the Go API. Context = repo root (apps/api is its own Go
# module; no other workspace files are needed at build time).
FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download

COPY apps/api .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------------------------------------------------------------------------
# Runtime: static Go binary + chromium (chromedp PDF rendering) + rendercv
# (Python/Typst CLI, exec'd for the RenderCV tailoring path) + poppler
# (pdftotext fallback for resume PDF import).
FROM python:3.12-slim-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
      chromium \
      poppler-utils \
      ca-certificates \
      fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

# rendercv ships the Typst binary transitively via its `typst` PyPI dependency
# — no separate Typst install needed. Isolated venv keeps it off the system
# Python (Debian's PEP 668 "externally managed" guard would otherwise block
# a bare `pip install`). Pinned to 2.8 (needs Python >=3.12) to match the
# schema our uploaded rendercv configs target (settings block, alpha2 locale).
RUN python3 -m venv /opt/rendercv-venv \
    && /opt/rendercv-venv/bin/pip install --no-cache-dir "rendercv[full]==2.8" click \
    && ln -s /opt/rendercv-venv/bin/rendercv /usr/local/bin/rendercv

COPY --from=build /out/server /usr/local/bin/server

ENV DOCUMENTS_DIR=/data/documents
ENV RENDERCV_BIN=/usr/local/bin/rendercv
# chromedp auto-detects a system Chromium via PATH lookup (tries
# "chromium", "chromium-browser", "google-chrome", ...) — the Debian
# `chromium` package installs it under that exact name, no extra flag needed.

EXPOSE 3000
# Goose migrations run automatically on startup (internal/db.Migrate),
# unlike the NestJS image's separate `drizzle-kit migrate` CMD step.
CMD ["/usr/local/bin/server"]
