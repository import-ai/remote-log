# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o remote-log .

# Runtime stage
FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/remote-log .

EXPOSE 8080

ENTRYPOINT ["./remote-log"]
