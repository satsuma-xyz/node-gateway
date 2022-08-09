# ⛩ node-gateway

Load balancer for blockchain nodes that provides better reliability and proper
data consistency.

## Quick start

#### Run with Docker

```sh
git clone https://github.com/satsuma-data/node-gateway.git
cd node-gateway
cp configs/config.sample.yml config.yml

docker run -d -p 8080:8080 \
  -v $PWD/config.yml:/etc/node-gateway/config.yml \
  -e "ALCHEMY_API_KEY=test_key" \
  satsuma-data/node-gateway:v0
```

## Configuration

See [sample config](/configs/config.sample.yml) here:

```yaml
global:
  port: 8080

# List of upstream node RPCs.
upstreams:
  # Each upstream can have the following keys:
  #
  # id - Unique identifier for the upstream.
  # chain - Chain name
  # url - RPC URL.
  - id: alchemy-eth
    chain: mainnet
    url: "https://eth-mainnet.g.alchemy.com/v2/${ALCHEMY_API_KEY}"
  - id: ankr-polygon
    chain: polygon
    url: "https://rpc.ankr.com/polygon"
  - id: my-node
    chain: bsc
    url: "http://12.57.207.168:8545"
```

## Features

- Round-robin load balancing for all EVM-based JSON RPCs.
- Health checks for block height, uptime, and response time.
- Automated routing to nodes at max block height for data consistency.

#### 🔮 Roadmap

- Better support for managed node providers (rate limits/throttling, authentication).
- Automatic retry / fallback.
- Caching.
- WebSockets.
- Better data consistency (verifying requests across multiple nodes, uncled blocks, etc).
- Additional routing strategies (archive/full node, primary/fallback, etc).
- Support for `eth_newBlockFilter`, `eth_newFilter`, and `eth_newPendingTransactionFilter`.

Interested in a specific feature? Join our [Discord community]() to let us know.

## Development

#### Running locally

1. Install Go 1.19.
2. Install dependencies: `go mod download`.
3. Run `go run cmd/gateway/main.go`.

#### Testing

Run tests with:

```sh
go build -v ./...
go test -v ./...
```

#### Linting

This project relies on [golangci-lint](https://github.com/golangci/golangci-lint) for linting. You can set up an [integration with your code editor](https://golangci-lint.run/usage/integrations/) to run lint checks locally.

#### Commit messages

All commit messages should follow the [conventional commit format](https://conventionalcommits.org).

For convenience of local development, there's a commit-msg Git hook that you can use:

```
ln -s $PWD/scripts/git_hooks/* .git/hooks
```

## License

This repo is licensed under the Apache License, Version 2.0. See [LICENSE]() for details.

Copyright © Riser Data, Inc.
