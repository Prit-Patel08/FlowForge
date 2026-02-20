# Build stage
FROM golang:1.25.7-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /flowforge .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /probe ./cmd/healthcheck

# Final stage
FROM gcr.io/distroless/static-debian12
WORKDIR /tmp
COPY --from=builder /flowforge /flowforge
COPY --from=builder /probe /probe
USER 65532:65532
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD ["/probe"]
ENTRYPOINT ["/flowforge", "run"]
