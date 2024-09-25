#!/bin/bash

# Ports to use for the servers
PORTS=(3333 4444 5555)

# Store the process IDs in an array
PIDS=()

# Function to shut down servers when Ctrl+C is pressed
function shutdown_servers {
    echo "Terminating servers..."
    local PID
    for PID in "${PIDS[@]}"; do
        kill "$PID"
    done
    wait "${PIDS[@]}" 2>/dev/null
    echo "Servers terminated."
    exit
}

# Trap Ctrl+C (SIGINT) and run shutdown_servers function
#
# Finding the PID of a process and killing it manually (if needed):
#   lsof -i :3333
#   kill <PID>
trap shutdown_servers SIGINT

# Start the servers on different ports
for PORT in "${PORTS[@]}"; do
    echo "Starting server on port $PORT"
    python3 -m server "$PORT" &  # Start each server in the background
    PIDS+=($!)  # Store the background process ID
done

# Wait for all background processes (the servers) to finish
wait
