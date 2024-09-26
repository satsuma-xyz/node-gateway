```zsh
# Start test servers.
./start_servers.sh

# Start node gateway with routing control ENABLED.
LOG_LEVEL=debug go run cmd/gateway/main.go config.yml

# Start node gateway with routing control DISABLED.
LOG_LEVEL=debug go run cmd/gateway/main.go config-disabled.yml
```