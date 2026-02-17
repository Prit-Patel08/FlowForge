# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
# Use -ldflags="-s -w" to shrink binary size
RUN CGO_ENABLED=0 GOOS=linux go build -o sentry -ldflags="-s -w" .

# Final stage
FROM gcr.io/distroless/static-debian12

WORKDIR /

# Copy the binary from the builder
COPY --from=builder /app/sentry /sentry

# Use non-root user for security
USER nonroot:nonroot

# Expose API port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/sentry", "run"]
