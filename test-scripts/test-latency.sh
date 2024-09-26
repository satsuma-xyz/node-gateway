#!/bin/bash

source test-scripts/common.sh

# This will cause both primary upstreams (ports 3333 & 4444) to be marked as unhealthy
# due to high latency rate. We need at least 3 failed requests to each upstream
# to trigger latency rate filtering, and we have 2 upstreams. The requests will round-robin
# between the two upstreams, so we need 6 failed requests in total.
# Note that unlike with error based routing, latency rate is kept separately for each method.
do_curl "eth_177" # Routed to 4444
do_curl "eth_177" # Routed to 3333
do_curl "eth_177" # Routed to 4444
do_curl "eth_177" # Routed to 3333
do_curl "eth_177" # Routed to 4444
do_curl "eth_177" # Routed to 3333

# Three slow requests to `eth_177` will trigger the latency rate filtering for upstream 5555.
# The fast `eth_77` will keep getting routed, since the requests always finish within our threshold of 100ms.
do_curl "eth_177" # Routed to 5555
do_curl "eth_77" # Routed to 3333
do_curl "eth_177" # Routed to 5555
do_curl "eth_77" # Routed to 3333
do_curl "eth_177" # Routed to 5555

# No healthy upstreams left, so the next request will not be routed.
do_curl "eth_177" # Not routed

# The fact that the upstream is kept separately for each method can be seen here,
# as routing for this method is unaffected by the routing of the other method.
do_curl "eth_77" # Routed to 3333
do_curl "eth_77" # Routed to 4444
do_curl "eth_77" # Routed to 3333
do_curl "eth_77" # Routed to 4444
