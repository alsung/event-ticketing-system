# Stage 1: Build the service
FROM golang:1.24 AS builder

WORKDIR /app
COPY . .
RUN go mod tidy
RUN go build -o user-service main.go

# Stage 2: Run the service
FROM gcr.io/distroless/base-debian10

WORKDIR /
COPY --from=builder /app/user-service /user-service

ENTRYPOINT [ "/user-service" ]