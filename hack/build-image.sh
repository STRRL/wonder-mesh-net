#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${PROJECT_ROOT}"

# Default tag: <commit-hash>(-dirty)
if [ -z "${IMAGE_TAG:-}" ]; then
    IMAGE_TAG=$(git rev-parse --short HEAD)
    if [[ $(git status --porcelain) ]]; then
        IMAGE_TAG="${IMAGE_TAG}-dirty"
    fi
fi

IMAGE_NAME="${IMAGE_NAME:-ghcr.io/strrl/wonder-mesh-net}"
PLATFORM="${PLATFORM:-}"

docker buildx inspect multiarch >/dev/null 2>&1 || \
    docker buildx create --use --name multiarch --driver docker-container

if [ -n "${PLATFORM}" ]; then
    echo "Building ${IMAGE_NAME}:${IMAGE_TAG} for ${PLATFORM}..."
    docker buildx build \
        --builder multiarch \
        --platform "${PLATFORM}" \
        -t "${IMAGE_NAME}:${IMAGE_TAG}" \
        --load \
        .
else
    echo "Building ${IMAGE_NAME}:${IMAGE_TAG}..."
    docker build -t "${IMAGE_NAME}:${IMAGE_TAG}" .
fi

echo "Done: ${IMAGE_NAME}:${IMAGE_TAG}"
