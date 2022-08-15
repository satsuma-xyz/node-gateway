# syntax=docker/dockerfile:1.4

FROM golang:1.19-alpine3.16 as builder

RUN apk add --no-cache make libc6-compat build-base

WORKDIR /app
ADD . .

RUN make build_binary

FROM docker.io/library/alpine:3.16

RUN mkdir /satsuma
COPY --from=builder /app/build/bin/* /usr/local/bin

EXPOSE 8080

CMD ["/usr/local/bin/gateway", "/satsuma/config.yml"]

ARG GIT_COMMIT_HASH=0
LABEL git_commit_hash=$GIT_COMMIT_HASH
