```zsh
# Start test servers.
./start_servers.sh

# Start node gateway.
LOG_LEVEL=debug go run cmd/gateway/main.go config.yml
```