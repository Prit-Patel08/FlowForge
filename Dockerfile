# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod and sum
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build the main binary
RUN CGO_ENABLED=0 GOOS=linux go build -o sentry -ldflags="-s -w" .

# Build a small healthcheck probe
RUN CGO_ENABLED=0 GOOS=linux go build -o probe agent-sentry/cmd/healthcheck

# Final stage
FROM gcr.io/distroless/static-debian12

WORKDIR /

# Copy binaries
COPY --from=builder /app/sentry /sentry
COPY --from=builder /app/probe /probe

# Use non-root user
USER nonroot:nonroot

# Expose API port
EXPOSE 8080

# Healthcheck using the native probe
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/probe"]

# Run as non-root
ENTRYPOINT ["/sentry", "run"]
