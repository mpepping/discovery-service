# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o discovery-service \
    ./cmd/discovery-service

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 discovery && \
    adduser -D -u 1000 -G discovery discovery

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/discovery-service /app/discovery-service

# Change ownership
RUN chown -R discovery:discovery /app

# Switch to non-root user
USER discovery

# Expose ports
EXPOSE 3000 3001 2122

# Set default command
ENTRYPOINT ["/app/discovery-service"]
CMD []
