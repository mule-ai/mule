# Multi-stage Dockerfile for Mule AI Platform

# Stage 1: Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o mule ./cmd/api

# Stage 2: Final stage with scratch
FROM scratch

# Copy CA certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary from builder stage
COPY --from=builder /app/mule /mule

# Expose port
EXPOSE 8080

# Set entrypoint
ENTRYPOINT ["/mule"]

# Default command with flags
CMD ["-db", "postgres://mule:mule@postgres:5432/mulev2?sslmode=disable", "-listen", ":8080"]