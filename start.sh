#!/bin/bash

echo "Starting Go application..."
go run main.go &

echo "Starting Bun application..."
bun redirector/index.js &

wait

echo "Both services have stopped."
