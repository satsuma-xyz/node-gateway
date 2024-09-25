#!/bin/bash

# Ports to use for the servers
PORTS=(3333 4444 5555)

# Function to shut down servers when Ctrl+C is pressed
function shutdown_servers {
    echo "Shutting down servers..."
    # Kill all background jobs (the python servers)
    kill "$(jobs -p)"
    exit 0
}

# Trap Ctrl+C (SIGINT) and run shutdown_servers function
trap shutdown_servers SIGINT

# Start the servers on different ports
for PORT in "${PORTS[@]}"; do
    echo "Starting server on port $PORT"
    python3 -m server "$PORT" &  # Start each server in the background
done

# Wait for all background processes (the servers) to finish
wait
