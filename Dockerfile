# ---- Builder Stage ----
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=1.0.0.0
ARG COMMIT=none
ARG MAIN_PKG=./cmd/app

ENV CGO_ENABLED=0
RUN mkdir -p /out
# trimpath + ldflags, strip for prod
RUN go build -trimpath -o /out/app \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    ${MAIN_PKG}

# ---- Final Stage ----
FROM alpine:3.19 AS runtime
RUN apk add --no-cache ca-certificates tzdata && update-ca-certificates

# Create non-root user
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app

COPY --from=builder /out/app /app/app
# Keep config path default (mounted by docker-compose)
ENV CONFIG_PATH=/etc/app/config.yaml
EXPOSE 8080

# Ensure ownership
RUN chown -R app:app /app
USER app

# Minimal HEALTHCHECK (adjust path as necessary)
# HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
#   CMD wget -qO- --header="Authorization: Bearer $ADMIN_API_KEY" http://127.0.0.1:8080/metrics >/dev/null || exit 1

ENTRYPOINT ["/app/app"]
CMD ["--config", "/etc/app/config.yaml"]