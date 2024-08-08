# ⛩ node-gateway

[![Test](https://github.com/satsuma-xyz/node-gateway/actions/workflows/test.yml/badge.svg)](https://github.com/satsuma-xyz/node-gateway/actions/workflows/test.yml)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/satsuma-xyz/node-gateway)](https://github.com/satsuma-xyz/node-gateway/releases)
[![Docker Image Version (latest by date)](https://img.shields.io/docker/v/satsumaxyz/node-gateway?logo=docker)](https://hub.docker.com/r/satsumaxyz/node-gateway/tags)
[![contributions welcome](https://img.shields.io/badge/contributions-welcome-brightgreen.svg?style=flat)](https://github.com/satsuma-xyz/node-gateway/issues)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://github.com/satsuma-xyz/node-gateway/blob/main/LICENSE)

An L7 load balancer for EVM-based blockchain nodes that provides better
reliability and proper data consistency.

## Introduction

Whether you're running your own nodes or using a managed provider, node RPCs
often go down or fall behind. Naive load balancing between nodes doesn't
account for [data consistency issues](https://alchemy.com/blog/data-accuracy).

node-gateway makes it easier to run reliable and accurate node infrastructure
for dApp developers, traders, and stakers.

## Example use cases

- Run your own nodes instead of paying for node providers. For very high availability, fall back on node providers in case your own nodes are unavailable. In our benchmarks, a single on-demand im4gn.4xlarge AWS EC2 machine that costs ~$1050 can serve over 1000 requests / second. This is > 10x cheaper than the well known node providers.
- Use a primary node provider and fall back on another node provider for even higher availability. The well known node providers tout 99.9% uptime (~9 hours of downtime a year) but often have degraded performance even if they're "up".

## Quick start

#### Run with Docker

```sh
git clone https://github.com/satsuma-data/node-gateway.git
cd node-gateway
cp configs/config.sample.yml config.yml

docker run -d -p 8080:8080 -p 9090:9090 \
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
- Health checks for block height and peer count.
- Automated routing to nodes at max block height for data consistency.
- Node groups with priority levels (e.g. primary/fallback).
- Multichain support.
- Intelligent routing to archive/full nodes based on type of JSON RPC request (state vs nonstate).
- Method based routing.
- Support for self-hosted nodes and node providers with basic authentication.
- Prometheus metrics.
- And much more!

#### 🔮 Roadmap

- Better support for managed node providers (e.g. rate limits/throttling).
- Automatic retry / fallback.
- Caching.
- WebSockets.
- Additional data consistency measures (broadcasting to multiple nodes, uncled blocks, etc).
- Additional routing strategies (intelligent routing to archive/full nodes based on recency of data requested, etc).
- Filter support (`eth_newBlockFilter`, `eth_newFilter`, and `eth_newPendingTransactionFilter`).

Interested in a specific feature? Join our [Telegram group chat](https://t.me/+9X-jV6P1z45hN2Ux) to let us know.

## Metrics

By default, Prometheus metrics are exposed on port 9090. See
[metrics.go](/internal/metrics/metrics.go) for more details.

## Development

#### Running locally

1. Install Go 1.22.
2. Install dependencies: `go mod download`.
3. Run `go run cmd/gateway/main.go`.

#### Testing

Generate mocks by first installing [mockery](https://github.com/vektra/mockery#installation), then running:

```sh
go generate ./...
```

This command will generate mocks for interfaces/types annotated with `go:generate mockery ...` and place them in the `mocks` folder

If you get a `command not found` error, make sure `~/go/bin` is in your `PATH`.

Run tests with:

```sh
go build -v ./...
go test -v ./...
```

To measure test code coverage, install the following tool and run the above `go test` command with the `-cover` flag:
```sh
go get golang.org/x/tools/cmd/cover
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
