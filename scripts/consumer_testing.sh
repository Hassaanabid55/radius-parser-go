#!/bin/bash

set -e

VHOST="radius"

echo "======================================="
echo " RabbitMQ Bootstrap Loader"
echo "======================================="

publish() {
  ROUTING_KEY="$1"
  PAYLOAD="$2"

  echo "$PAYLOAD" | rabbitmqadmin publish \
    --vhost "$VHOST" \
    "$ROUTING_KEY"
}

echo "[1/2] Sending CGNAT entries..."

publish "cgnat.load" '{
  "type": "cgnat.load",
  "payload": {
    "entries": [
      {"InsideIP":"10.250.41.153","NatIP":"5.38.72.0","StartPort":1,"EndPort":6666},
      {"InsideIP":"10.11.5.0","NatIP":"5.38.72.0","StartPort":1024,"EndPort":2977},
      {"InsideIP":"10.11.6.0","NatIP":"5.38.72.0","StartPort":2978,"EndPort":4931},
      {"InsideIP":"10.11.7.0","NatIP":"5.38.72.0","StartPort":4932,"EndPort":6885},
      {"InsideIP":"10.11.9.0","NatIP":"5.38.72.0","StartPort":6886,"EndPort":8839}
    ]
  }
}'

echo "[2/2] Sending whitelist entries..."

publish "whitelist.load" '{
  "type": "whitelist.load",
  "payload": {
    "entries": [
      {"MSISDN":"971566895806","Status":false},
      {"MSISDN":"971506966062","Status":false},
      {"MSISDN":"971504069617","Status":false},
      {"MSISDN":"971501060525","Status":true},
      {"MSISDN":"971505907950","Status":true},
      {"MSISDN":"971562686223","Status":true}
    ]
  }
}'

echo "======================================="
echo " DONE"
echo "======================================="