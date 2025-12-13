#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

IMAGE_NAME="${IMAGE_NAME:-ghcr.io/strrl/wonder-mesh-net}"
IMAGE_TAG="${IMAGE_TAG:-dev}"

cd "${PROJECT_ROOT}"

echo "Building ${IMAGE_NAME}:${IMAGE_TAG} for linux/amd64,linux/arm64..."

docker buildx build \
    --platform linux/amd64,linux/arm64 \
    -t "${IMAGE_NAME}:${IMAGE_TAG}" \
    --push \
    .

echo "Pushed: ${IMAGE_NAME}:${IMAGE_TAG}"
