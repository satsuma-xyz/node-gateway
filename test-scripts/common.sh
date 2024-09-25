#!/bin/bash

do_curl() {
    local METHOD=$1
    curl --request POST \
     --url http://localhost:8080/mainnet \
     --header 'accept: application/json' \
     --header 'content-type: application/json' \
     --data "{
  \"id\": 1,
  \"jsonrpc\": \"2.0\",
  \"method\": \"$METHOD\",
  \"params\": []
}"
}

do_curl "eth_slowMethod"
