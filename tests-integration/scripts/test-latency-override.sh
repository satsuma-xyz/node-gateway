#!/bin/bash

source test-scripts/common.sh

# Even though the slow method takes longer than 100ms, it will not be routed to the secondary upstream
# since the latency threshold override for this method is 2000ms.
do_curl "eth_slowMethod" # Routed to 4444
do_curl "eth_slowMethod" # Routed to 3333
do_curl "eth_slowMethod" # Routed to 4444
do_curl "eth_slowMethod" # Routed to 3333
do_curl "eth_slowMethod" # Routed to 4444
do_curl "eth_slowMethod" # Routed to 3333
do_curl "eth_slowMethod" # Routed to 4444
do_curl "eth_slowMethod" # Routed to 3333
do_curl "eth_slowMethod" # Routed to 4444
do_curl "eth_slowMethod" # Routed to 3333
do_curl "eth_slowMethod" # Routed to 4444
do_curl "eth_slowMethod" # Routed to 3333
