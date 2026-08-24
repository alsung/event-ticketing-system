# Build context is ./services (not this directory), because every service go.mod
# carries a `replace ... => ../pkg` directive pointing at the shared module. With
# a per-service context that path sits outside the build and `go mod download`
# cannot resolve it.
FROM golang:1.25 AS builder

WORKDIR /src
COPY pkg/ ./pkg/
COPY user-service/ ./user-service/

WORKDIR /src/user-service
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/user-service ./cmd/main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/user-service /user-service
EXPOSE 8081
ENTRYPOINT ["/user-service"]
