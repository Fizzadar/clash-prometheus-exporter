#!/bin/sh

docker buildx build \
    --no-cache \
    --push \
    --platform linux/amd64,linux/arm64 \
    -t fizzadar/clash-prometheus-exporter:0.2 \
    .
