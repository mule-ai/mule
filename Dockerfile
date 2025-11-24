# Multi-stage Dockerfile for Mule AI Platform

# Stage 1: Frontend build stage
FROM node:18-alpine AS frontend-builder

# Set working directory
WORKDIR /app

# Copy frontend package files
COPY frontend/package*.json ./

# Install frontend dependencies
RUN npm install

# Copy frontend source code
COPY frontend/ .

# Build the frontend
RUN npm run build

# Stage 2: Backend build stage
FROM golang:1.25-alpine AS builder

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

# Copy built frontend files to the internal/frontend/build directory
COPY --from=frontend-builder /app/build ./internal/frontend/build

# Run tests before building
RUN go test ./...

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o mule ./cmd/api

# Stage 3: Final stage with alpine
FROM alpine:latest

# Install ca-certificates for HTTPS requests and Go toolchain for WASM compilation
RUN apk --no-cache add ca-certificates tzdata go git musl-dev

# Create non-root user
RUN adduser -D -s /bin/sh mule

# Copy the binary from builder stage
COPY --from=builder /app/mule /usr/local/bin/mule

# Change ownership to non-root user
RUN chown mule:mule /usr/local/bin/mule

# Switch to non-root user
USER mule

# Expose port
EXPOSE 8080

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/mule"]

# Default command with flags
CMD ["-db", "postgres://mule:mule@postgres:5432/mulev2?sslmode=disable", "-listen", ":8080"]
