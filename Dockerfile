# Build stage
FROM golang:1.21-alpine AS builder

# Install dependencies
RUN apk add --no-cache git ca-certificates

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
    -ldflags="-w -s -X 'main.Version=$(git describe --tags --always --dirty)'" \
    -a -installsuffix cgo -o fanet-api cmd/fanet-api/main.go

# Runtime stage
FROM alpine:3.19

# Install ca-certificates and timezone data
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S fanet && \
    adduser -u 1000 -S fanet -G fanet

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/fanet-api .

# Copy any static files if needed
# COPY --from=builder /build/static ./static

# Change ownership
RUN chown -R fanet:fanet /app

# Switch to non-root user
USER fanet

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the binary
CMD ["./fanet-api"]