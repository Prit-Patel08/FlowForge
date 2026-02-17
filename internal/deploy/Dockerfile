# Build stage
FROM golang:1.21 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /sentry ./cmd

# Final stage
FROM gcr.io/distroless/static-debian12
COPY --from=builder /sentry /sentry
USER 65532:65532
ENTRYPOINT ["/sentry", "run"]
