#!/bin/bash

# Quick start script for API Gateway

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

echo "🏗️  Building API Gateway..."
go build -o api-gateway .

if [ $? -ne 0 ]; then
    echo "❌ Build failed"
    exit 1
fi

echo "✅ Build successful"
echo ""

# Check if ports are already in use
check_port() {
    if nc -z localhost $1 2>/dev/null; then
        echo "⚠️  Port $1 is already in use. Kill existing process? (y/n)"
        read -r response
        if [ "$response" = "y" ]; then
            lsof -ti :$1 | xargs kill -9 2>/dev/null || true
        else
            return 1
        fi
    fi
    return 0
}

# Start services in background
start_service() {
    local port=$1
    local name=$2
    local mode=$3
    
    if ! check_port $port; then
        return 1
    fi
    
    echo "Starting $name on port $port..."
    ./api-gateway -mode $mode -port $port -name "$name" > "${name}.log" 2>&1 &
    local pid=$!
    echo "  PID: $pid"
    sleep 1
    return 0
}

echo "🚀 Starting services..."
echo ""

if start_service 8080 "Gateway" "gateway"; then
    gateway_pid=$!
fi

if start_service 8081 "Backend-1" "backend"; then
    backend1_pid=$!
fi

if start_service 8082 "Backend-2" "backend"; then
    backend2_pid=$!
fi

sleep 2

echo ""
echo "✅ All services started!"
echo ""
echo "Gateway:  http://localhost:8080"
echo "Backend1: http://localhost:8081"
echo "Backend2: http://localhost:8082"
echo ""
echo "Test commands:"
echo "  go run . -mode client -cmd health -endpoint http://localhost:8080"
echo "  go run . -mode client -cmd user -endpoint http://localhost:8080 -count 5"
echo "  go run . -mode client -cmd auth -endpoint http://localhost:8080"
echo ""
echo "View logs:"
echo "  tail -f gateway.log"
echo "  tail -f Gateway.log"
echo "  tail -f Backend-1.log"
echo "  tail -f Backend-2.log"
echo ""
echo "Stop services:"
echo "  pkill -f api-gateway"
echo ""

# Keep script running
wait
