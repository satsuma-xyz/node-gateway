```zsh
# Start test servers.
# TODO(polsar): Ports are not immediately available after stopping servers.
./start_servers.sh

# Start node gateway with routing control DISABLED.
LOG_LEVEL=debug go run cmd/gateway/main.go config-disabled.yml

# Start node gateway with routing control ENABLED.
LOG_LEVEL=debug go run cmd/gateway/main.go config.yml

# Start node gateway with routing control ENABLED and with `alwaysRoute` option.
LOG_LEVEL=debug go run cmd/gateway/main.go config-always-route.yml

# Run tests. Git should show no diff after running these tests.
# YOU MUST RESTART THE GATEWAY AFTER EACH TEST RUN TO CLEAR THE STATE!
# TODO(polsar): Add a test script for error rate routing based on matching JSON RPC codes.
./test-scripts/test-http-error.sh > test-scripts-output-enabled/test-http-error.txt
./test-scripts/test-http-error.sh > test-scripts-output-enabled/test-http-error-always-route.txt

./test-scripts/test-error-string.sh
./test-scripts/test-error-string-and-http.sh
./test-scripts/test-latency.sh
./test-scripts/test-latency-override.sh
```