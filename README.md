# ⛩ node-gateway
[![Test](https://github.com/satsuma-xyz/node-gateway/actions/workflows/test.yml/badge.svg)](https://github.com/satsuma-xyz/node-gateway/actions/workflows/test.yml)

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
  -v $PWD/config.yml:/satsuma/config.yml \
  -e ENV=production \
  satsumaxyz/node-gateway:latest
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
- Health checks for block height, peer count, and sync status.
- Automated routing to nodes at max block height for data consistency.
- Node grouping which supports priority load balancing (e.g. primary/fallback).

#### 🔮 Roadmap

- Better support for managed node providers (rate limits/throttling, authentication).
- Automatic retry / fallback.
- Monitoring (metrics, UI dashboard).
- Caching.
- WebSockets.
- Additional data consistency measures (broadcasting to multiple nodes, uncled blocks, etc).
- Additional routing strategies (archive/full node, etc).
- Filter support (`eth_newBlockFilter`, `eth_newFilter`, and `eth_newPendingTransactionFilter`).

Interested in a specific feature? Join our [Discord community]() to let us know.

## Metrics

By default, Prometheus metrics are exposed on port 9090. See
[metrics.go](/internal/metrics/metrics.go) for more details.

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
