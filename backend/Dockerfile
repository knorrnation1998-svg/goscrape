# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /goscrape-server ./cmd/server

# Final lightweight stage
FROM alpine:latest
WORKDIR /app

COPY --from=builder /goscrape-server /app/goscrape-server

EXPOSE 8080

CMD ["/app/goscrape-server", "-addr", "0.0.0.0:8080", "-db", "/app/data/goscrape.db"]