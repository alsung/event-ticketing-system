# Build context is ./services — see the note in user-service/Dockerfile.
FROM golang:1.25 AS builder

WORKDIR /src
COPY pkg/ ./pkg/
COPY event-service/ ./event-service/

WORKDIR /src/event-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/event-service ./cmd/main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/event-service /event-service
EXPOSE 8082
ENTRYPOINT ["/event-service"]
