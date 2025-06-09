#!/bin/bash

# Post-create script for FANET API Backend development container
set -e

echo "🚀 Setting up FANET API Backend development environment..."

# Ensure we're in the right directory
cd /workspace

# Install Go dependencies
echo "📦 Installing Go dependencies..."
go mod download
go mod tidy

# Generate Protocol Buffers if needed
echo "🔄 Generating Protocol Buffers..."
if [ -f "scripts/proto-gen.sh" ]; then
    chmod +x scripts/proto-gen.sh
    ./scripts/proto-gen.sh
else
    # Fallback protobuf generation
    if [ -d "ai-spec/api" ]; then
        mkdir -p pkg/pb
        protoc --go_out=pkg/pb --go_opt=paths=source_relative \
               --go-grpc_out=pkg/pb --go-grpc_opt=paths=source_relative \
               ai-spec/api/*.proto 2>/dev/null || echo "⚠️  No proto files found or generation failed"
    fi
fi

# Install additional Go tools specific to this project
echo "🛠️  Installing project-specific tools..."
# Air уже установлен в Dockerfile
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Set up Git hooks if they exist
if [ -d ".git/hooks" ]; then
    echo "🔗 Setting up Git hooks..."
    chmod +x .git/hooks/* 2>/dev/null || true
fi

# Create necessary directories
echo "📁 Creating necessary directories..."
mkdir -p tmp bin logs

# Make scripts executable
echo "🔧 Making scripts executable..."
chmod +x scripts/*.sh 2>/dev/null || true
chmod +x deployments/docker/*.sh 2>/dev/null || true

# Set up environment file if it doesn't exist
if [ ! -f ".env" ]; then
    echo "📄 Creating .env file..."
    cat > .env << 'EOL'
# FANET API Backend Development Environment
SERVER_PORT=8090
REDIS_URL=redis://redis:6379
MQTT_URL=tcp://mqtt:1883
MYSQL_DSN=root:password@tcp(mysql:3306)/fanet?parseTime=true
AUTH_ENDPOINT=https://api.flybeeper.com/api/v4/user
AUTH_CACHE_TTL=5m
DEFAULT_RADIUS_KM=200
LOG_LEVEL=debug

# Development flags
DEBUG=true
ENABLE_METRICS=true
ENABLE_PPROF=true
EOL
fi

# Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
timeout=60
elapsed=0

# Wait for Redis
while ! redis-cli -h redis ping >/dev/null 2>&1; do
    if [ $elapsed -ge $timeout ]; then
        echo "❌ Redis is not ready after ${timeout}s"
        break
    fi
    echo "⏳ Waiting for Redis... (${elapsed}s)"
    sleep 2
    elapsed=$((elapsed + 2))
done

# Wait for MQTT
while ! mosquitto_pub -h mqtt -t test -m test >/dev/null 2>&1; do
    if [ $elapsed -ge $timeout ]; then
        echo "❌ MQTT is not ready after ${timeout}s"
        break
    fi
    echo "⏳ Waiting for MQTT... (${elapsed}s)"
    sleep 2
    elapsed=$((elapsed + 2))
done

# Wait for MySQL
while ! mysqladmin ping -h mysql -u root -ppassword >/dev/null 2>&1; do
    if [ $elapsed -ge $timeout ]; then
        echo "❌ MySQL is not ready after ${timeout}s"
        break
    fi
    echo "⏳ Waiting for MySQL... (${elapsed}s)"
    sleep 2
    elapsed=$((elapsed + 2))
done

# Run initial build to check everything works
echo "🔨 Running initial build check..."
if go build -o bin/fanet-api cmd/fanet-api/main.go; then
    echo "✅ Build successful!"
else
    echo "❌ Build failed - check your code"
fi

# Display useful information
echo ""
echo "🎉 Development environment setup complete!"
echo ""
echo "📋 Available services:"
echo "  • FANET API:       http://localhost:8090"
echo "  • Redis:           redis://localhost:6379"
echo "  • Redis Commander: http://localhost:8081"
echo "  • MQTT Broker:     mqtt://localhost:1883"
echo "  • MySQL:           mysql://localhost:3306"
echo "  • Adminer:         http://localhost:8082"
echo "  • Prometheus:      http://localhost:9090"
echo "  • Grafana:         http://localhost:3000 (admin/admin)"
echo ""
echo "🚀 Quick start commands:"
echo "  make dev           # Start API with hot reload"
echo "  make test          # Run tests"
echo "  make mqtt-test     # Test MQTT integration"
echo "  make proto         # Regenerate protobuf"
echo ""
echo "📖 See DEVELOPMENT.md for detailed documentation"