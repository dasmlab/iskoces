#!/usr/bin/env bash
# build-and-load-images.sh
# Builds Iskoces image for the current architecture and pushes to ghcr.io
# Images are tagged with architecture-specific tags (e.g., local-arm64, local-amd64)

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Detect architecture
ARCH=$(uname -m)
case "${ARCH}" in
    arm64|aarch64)
        ARCH_TAG="arm64"
        ;;
    x86_64|amd64)
        ARCH_TAG="amd64"
        ;;
    *)
        log_warn "Unknown architecture: ${ARCH}, defaulting to 'unknown'"
        ARCH_TAG="unknown"
        ;;
esac

log_info "Detected architecture: ${ARCH} (tag: ${ARCH_TAG})"

# Registry configuration
REGISTRY="ghcr.io/dasmlab"
IMAGE_NAME="iskoces-server"
IMAGE_TAG="${REGISTRY}/${IMAGE_NAME}:local-${ARCH_TAG}"

# Check for GitHub token
GHCR_PAT="${DASMLAB_GHCR_PAT:-}"
if [ -z "${GHCR_PAT}" ]; then
    log_error "DASMLAB_GHCR_PAT environment variable is required"
    log_info "Set it with: export DASMLAB_GHCR_PAT=your_token"
    log_info "The token should be a GitHub PAT with 'write:packages' permission"
    exit 1
fi

# Authenticate with GitHub Container Registry
log_info "Authenticating with GitHub Container Registry..."
echo "${GHCR_PAT}" | docker login ghcr.io -u lmcdasm --password-stdin || {
    log_error "Failed to authenticate with ghcr.io"
    exit 1
}
log_success "Authenticated with ghcr.io"

log_info "Building and pushing Iskoces image for architecture: ${ARCH_TAG}..."

# Build image
log_info "Building Iskoces image..."
cd "${PROJECT_ROOT}"

# Use buildme.sh if available, otherwise use docker build directly
if [ -f "./buildme.sh" ]; then
    # buildme.sh builds with tag "scratch", we'll retag
    ./buildme.sh || {
        log_error "Failed to build Iskoces image"
        exit 1
    }
    docker tag "${IMAGE_NAME}:scratch" "${IMAGE_TAG}" || {
        log_error "Failed to tag Iskoces image"
        exit 1
    }
else
    # Fallback to docker build
    log_info "Using docker build (buildme.sh not found)"
    docker build -t "${IMAGE_TAG}" . || {
        log_error "Failed to build Iskoces image"
        exit 1
    }
fi
log_success "Iskoces image built: ${IMAGE_TAG}"

# Push image
log_info "Pushing Iskoces image to registry..."
docker push "${IMAGE_TAG}" || {
    log_error "Failed to push Iskoces image"
    exit 1
}
log_success "Iskoces image pushed: ${IMAGE_TAG}"

log_success "Iskoces image built and pushed successfully!"
log_info "Image available in registry: ${IMAGE_TAG}"
echo ""
log_info "Deployment manifest should use this image:"
echo "  image: ${IMAGE_TAG}"

