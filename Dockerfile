# ---- Builder (Debian) ----
FROM golang:1.24-bookworm AS builder
WORKDIR /src

# Install build deps
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build static binary
COPY . .
ENV CGO_ENABLED=0
RUN go build -o /out/app -ldflags "-s -w" ./cmd/app

# ---- Runtime (Debian slim) ----
FROM debian:bookworm-slim

# Install minimal runtime deps: CA certs for TLS, tzdata for correct time, curl for healthcheck
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata curl && rm -rf /var/lib/apt/lists/*

# Non-root user
RUN useradd --system --home /app --shell /usr/sbin/nologin app
USER app

WORKDIR /app
COPY --from=builder /out/app /app/app

# Config path
ENV CONFIG_PATH=/etc/app/config.yaml

# HTTP server port
EXPOSE 8080

# Healthcheck hits /metrics
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/metrics >/dev/null || exit 1

ENTRYPOINT ["/app/app"]
CMD ["--config", "/etc/app/config.yaml"]
