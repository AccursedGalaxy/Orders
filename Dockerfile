# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the applications
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/streamer cmd/streamer/main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/redis-viewer scripts/redis-viewer.go

# Final stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binaries from builder
COPY --from=builder /app/bin/streamer /app/streamer
COPY --from=builder /app/bin/redis-viewer /app/redis-viewer

# Copy configuration files
COPY .env.example /app/.env

# Set environment variables
ENV CUSTOM_REDIS_URL=redis://redis:6379/0

# Expose ports
EXPOSE 8080

# Set entrypoint
ENTRYPOINT ["/app/streamer"] 