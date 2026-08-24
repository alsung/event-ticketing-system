# Build context is ./services — see the note in user-service/Dockerfile.
FROM golang:1.25 AS builder

WORKDIR /src
COPY pkg/ ./pkg/
COPY api-gateway/ ./api-gateway/

WORKDIR /src/api-gateway
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api-gateway ./cmd/main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/api-gateway /api-gateway
EXPOSE 8000
ENTRYPOINT ["/api-gateway"]
