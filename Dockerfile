# ---- Builder Stage ----
# Use a specific Go version and a minimal OS (alpine) for a smaller builder.
FROM golang:1.24-alpine AS builder

# Install build dependencies. For Go, only ca-certificates are typically needed.
# git is only required if you have private modules or non-standard module versions.
RUN apk add --no-cache ca-certificates

WORKDIR /src

# Copy only the module files first to leverage Docker's layer caching.
# This layer only gets rebuilt if go.mod or go.sum changes.
COPY go.mod go.sum ./
RUN go mod download

# OPTIMIZATION: Copy source code *after* downloading dependencies.
# This ensures that code changes don't cause a re-download of all modules.
COPY . .

# Add ARGs for build-time variables, allowing us to pass version info.
ARG VERSION=dev
ARG COMMIT=none

# Build a static, stripped binary with version information baked in.
ENV CGO_ENABLED=0
RUN go build -o /out/app -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" ./cmd/app


# ---- Final Stage ----
# Use a distroless static image for the final stage. It contains only our app,
# its dependencies, and essential libraries like ca-certificates and tzdata.
# It has no shell or other programs, drastically reducing the attack surface.
FROM gcr.io/distroless/static-debian12

# Non-root user for security. Distroless images have a 'nonroot' user by default (uid 65532).
USER nonroot

WORKDIR /app
COPY --from=builder /out/app /app/app

# Config path and exposed port remain the same.
ENV CONFIG_PATH=/etc/app/config.yaml
EXPOSE 8080

# NOTE: The standard HEALTHCHECK cannot use `curl` as it doesn't exist in a distroless image.
# Docker's built-in health check can be configured to check the port directly if needed,
# or you can build a small health-check utility into your Go application itself.
# For now, we'll omit it in favor of the significant security gain.

ENTRYPOINT ["/app/app"]
CMD ["--config", "/etc/app/config.yaml"]