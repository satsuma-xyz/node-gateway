#!/bin/bash

source test-scripts/common.sh

# This will cause both primary upstreams (ports 3333 & 4444) to be marked as unhealthy
# due to high error rate. We need at least 3 failed requests to each upstream
# to trigger error rate filtering, and we have 2 upstreams. The requests will round-robin
# between the two upstreams, so we need 6 failed requests in total.
do_curl "eth_404" # Routed to 4444
do_curl "eth_404" # Routed to 3333
do_curl "eth_404" # Routed to 4444
do_curl "eth_404" # Routed to 3333
do_curl "eth_404" # Routed to 4444
do_curl "eth_404" # Routed to 3333
# These requests will be routed to the (single) secondary upstream on port 5555,
# since the primary upstreams are unhealthy now (the 4th request for each upstream
# triggers the error rate filtering).
do_curl "eth_anotherMethod" # Routed to 5555
# Even though a failed request, this will not count towards the error rate since
# the returned status code is 400, not 404.
do_curl "eth_400" # Routed to 5555
do_curl "eth_method" # Routed to 5555
# This request does count towards the error rate, since the status code is 404.
# This will bring the error rate of the secondary upstream to 0.25, marking it as
# unhealthy.
do_curl "eth_404" # Routed to 5555

# There are no healthy upstreams left, so the next request will not be routed.
# IF `alwaysRoute` IS SET TO `true`, THE REQUEST WILL BE ROUTED TO 3333.
do_curl "eth_method" # Not routed

# IF YOU START NODE GATEWAY WITH ROUTING CONTROL DISABLED, THE REQUESTS WILL SIMPLY
# ROUND-ROBIN BETWEEN THE 4444 & 3333 UPSTREAMS.
