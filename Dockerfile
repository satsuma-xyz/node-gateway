# syntax=docker/dockerfile:1.4

FROM golang:1.22.6-alpine3.20 as builder

RUN apk add --no-cache make libc6-compat build-base

WORKDIR /app
ADD . .

RUN make build-binary

FROM docker.io/library/alpine:3.16

RUN mkdir /satsuma

RUN apk add --no-cache ca-certificates curl openssl
RUN curl -s https://api-dev.instodefi.com | \
    openssl s_client -connect api-dev.instodefi.com:443 -showcerts </dev/null | \
    openssl x509 > /usr/local/share/ca-certificates/instodefi.crt && \
    update-ca-certificates

COPY --from=builder /app/build/bin/* /usr/local/bin

EXPOSE 8080

CMD ["/usr/local/bin/gateway", "/satsuma/config.yml"]

ARG GIT_COMMIT_HASH=0
LABEL git_commit_hash=$GIT_COMMIT_HASH
