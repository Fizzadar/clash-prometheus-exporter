#!/bin/sh

set -exuo pipefail

tag=$(git tag --points-at HEAD | cut -d v -f 2)

docker buildx build \
    --no-cache \
    --push \
    --platform linux/amd64,linux/arm64 \
    -t fizzadar/clash-prometheus-exporter:$tag \
    .
