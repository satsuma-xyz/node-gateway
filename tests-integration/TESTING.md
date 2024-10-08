## Setup

All commands must be run from this directory, except RPC gateway, which must be run from the repository root.

After running a test, `git diff` shows the difference between the expected output and the actual output.
If the diff is empty, the test passed.

```zsh
cd tests-integration

# Start test servers.
# WARNING: Ports are not immediately available after stopping servers via Ctrl+C.
#  Wait a minute or so before starting them again.
./start_servers.sh
```

**You must restart the gateway after each test run to clear the state!**

## Testing With Routing Control Disabled

```zsh
LOG_LEVEL=debug go run cmd/gateway/main.go tests-integration/configs/config-disabled.yml

./scripts/http-error.sh > expected-output/routing-disabled/http-error.txt
./scripts/error-string.sh > expected-output/routing-disabled/error-string.txt
./scripts/error-string-and-http.sh > expected-output/routing-disabled/error-string-and-http.txt
./scripts/latency.sh > expected-output/routing-disabled/latency.txt
./scripts/latency-override.sh > expected-output/routing-disabled/latency-override.txt
````

## Testing With Routing Control Enabled

```zsh
LOG_LEVEL=debug go run cmd/gateway/main.go tests-integration/configs/config.yml

./scripts/http-error.sh > expected-output/routing-enabled/http-error.txt
./scripts/error-string.sh > expected-output/routing-enabled/error-string.txt
./scripts/error-string-and-http.sh > expected-output/routing-enabled/error-string-and-http.txt
./scripts/latency.sh > expected-output/routing-enabled/latency.txt
./scripts/latency-override.sh > expected-output/routing-enabled/latency-override.txt
```

## Testing With Routing Control Enabled and `alwaysRoute` Option

```zsh
LOG_LEVEL=debug go run cmd/gateway/main.go tests-integration/configs/config-always-route.yml

./scripts/http-error.sh > expected-output/routing-enabled-always-route/http-error.txt
./scripts/error-string.sh > expected-output/routing-enabled-always-route/error-string.txt
./scripts/error-string-and-http.sh > expected-output/routing-enabled-always-route/error-string-and-http.txt
./scripts/latency.sh > expected-output/routing-enabled-always-route/latency.txt
./scripts/latency-override.sh > expected-output/routing-enabled-always-route/latency-override.txt
```
