#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${PROJECT_ROOT}"

# Compute version: tag if tagged, "untagged" otherwise; sha with -dirty suffix if dirty
GIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_TAG=$(git describe --tags --exact-match 2>/dev/null || echo "")

if [[ -n "${GIT_TAG}" ]]; then
    VERSION="${GIT_TAG}"
else
    VERSION="untagged"
fi

if [[ -n $(git status --porcelain 2>/dev/null) ]]; then
    GIT_SHA="${GIT_SHA}-dirty"
fi

# Allow override via environment
VERSION="${VERSION_OVERRIDE:-${VERSION}}"
GIT_SHA="${GIT_SHA_OVERRIDE:-${GIT_SHA}}"

# Default image tag: use VERSION if not specified
if [ -z "${IMAGE_TAG:-}" ]; then
    IMAGE_TAG="${VERSION}"
fi

IMAGE_NAME="${IMAGE_NAME:-ghcr.io/strrl/wonder-mesh-net}"
PLATFORM="${PLATFORM:-}"

echo "Version: ${VERSION}"
echo "Image tag: ${IMAGE_TAG}"

docker buildx inspect multiarch >/dev/null 2>&1 || \
    docker buildx create --use --name multiarch --driver docker-container

BUILD_ARGS="--build-arg VERSION=${VERSION} --build-arg GIT_SHA=${GIT_SHA}"

if [ -n "${PLATFORM}" ]; then
    echo "Building ${IMAGE_NAME}:${IMAGE_TAG} for ${PLATFORM}..."
    docker buildx build \
        --builder multiarch \
        --platform "${PLATFORM}" \
        ${BUILD_ARGS} \
        -t "${IMAGE_NAME}:${IMAGE_TAG}" \
        --load \
        .
else
    echo "Building ${IMAGE_NAME}:${IMAGE_TAG}..."
    docker build ${BUILD_ARGS} -t "${IMAGE_NAME}:${IMAGE_TAG}" .
fi

echo "Done: ${IMAGE_NAME}:${IMAGE_TAG}"
