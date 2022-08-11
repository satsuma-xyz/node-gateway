# ⛩ node-gateway

A L7 load balancer for blockchain nodes that provides better reliability and
proper data consistency.

## Introduction

Whether you're running your own nodes or using a managed provider, node RPCs
often go down or fall behind. Naive load balancing between nodes doesn't
account for [data consistency issues](https://alchemy.com/blog/data-accuracy).

node-gateway makes it easier to run reliable and accurate node infrastructure
for serving dApps, trading, and staking.

## Quick start

#### Run with Docker

```sh
git clone https://github.com/satsuma-data/node-gateway.git
cd node-gateway
cp configs/config.sample.yml config.yml

docker run -d -p 8080:8080 \
  -v $PWD/config.yml:/etc/node-gateway/configs/config.yml \
  satsuma-data/node-gateway:v0
```

#### Usage

```sh
curl --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' http://localhost:8080

{"jsonrpc":"2.0","id":1,"result":"0xe9a0a1"}
```

## Configuration

See the [sample config](/configs/config.sample.yml).

## Features

- Round-robin load balancing for EVM-based JSON RPCs.
- Health checks for block height, uptime, and response time.
- Automated routing to nodes at max block height for data consistency.

#### 🔮 Roadmap

- Better support for managed node providers (rate limits/throttling, authentication).
- Automatic retry / fallback.
- Monitoring (metrics, UI dashboard).
- Caching.
- WebSockets.
- Additional data consistency measures (broadcasting to multiple nodes, uncled blocks, etc).
- Additional routing strategies (archive/full node, primary/fallback, etc).
- Filter support (`eth_newBlockFilter`, `eth_newFilter`, and `eth_newPendingTransactionFilter`).

Interested in a specific feature? Join our [Discord community]() to let us know.

## Development

#### Running locally

1. Install Go 1.19.
2. Install dependencies: `go mod download`.
3. Run `go run cmd/gateway/main.go`.

#### Testing

Generate mocks by first installing [mockery](https://github.com/vektra/mockery#installation), then running:
```sh
go generate ./...
```
This command will generate mocks for interfaces/types annotated with `go:generate mockery ...` and place them in the `mocks` folder

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
