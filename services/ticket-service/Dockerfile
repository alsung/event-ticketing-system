# Build context is ./services — see the note in user-service/Dockerfile.
FROM golang:1.25 AS builder

WORKDIR /src
COPY pkg/ ./pkg/
COPY ticket-service/ ./ticket-service/

WORKDIR /src/ticket-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ticket-service ./cmd/main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/ticket-service /ticket-service
EXPOSE 8083
# The runtime image is distroless: no shell, no curl. The binary probes itself.
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/ticket-service", "-healthcheck"]

ENTRYPOINT ["/ticket-service"]
