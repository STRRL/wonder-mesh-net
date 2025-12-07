#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

IMAGE_NAME="${IMAGE_NAME:-ghcr.io/strrl/wonder-mesh-net}"
IMAGE_TAG="${IMAGE_TAG:-dev}"

cd "${PROJECT_ROOT}"

echo "Building ${IMAGE_NAME}:${IMAGE_TAG}..."

DOCKER_BUILDKIT=1 docker build \
    -t "${IMAGE_NAME}:${IMAGE_TAG}" \
    -f Dockerfile \
    .

echo "Done: ${IMAGE_NAME}:${IMAGE_TAG}"
