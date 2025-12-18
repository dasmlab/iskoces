#!/usr/bin/env bash
# release_images.sh
# Builds and pushes Iskoces images with the :released tag
# This tag represents the latest release and is used by the user install script
# Run this script manually when creating a new release

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Script directory (project root)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}"

# Release tag
RELEASE_TAG="released"

# Registry configuration
REGISTRY="ghcr.io/dasmlab"
ISKOCES_IMG="${REGISTRY}/iskoces-server:${RELEASE_TAG}"

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

log_step() {
    echo ""
    echo "=========================================="
    echo -e "${BLUE}$1${NC}"
    echo "=========================================="
    echo ""
}

# Check for GitHub token (try to source from standard locations if not set)
if [ -z "${DASMLAB_GHCR_PAT:-}" ]; then
    # Try /Users/dasm/gh_token (primary location for macOS)
    if [ -f "/Users/dasm/gh_token" ]; then
        export DASMLAB_GHCR_PAT="$(cat "/Users/dasm/gh_token" | tr -d '\n\r ')"
    # Try ~/gh-pat (bash script)
    elif [ -f "${HOME}/gh-pat" ]; then
        source "${HOME}/gh-pat" 2>/dev/null || true
    # Try ~/gh-pat/token (plain token file)
    elif [ -f "${HOME}/gh-pat/token" ]; then
        export DASMLAB_GHCR_PAT="$(cat "${HOME}/gh-pat/token" | tr -d '\n\r ')"
    fi
fi

GHCR_PAT="${DASMLAB_GHCR_PAT:-}"
if [ -z "${GHCR_PAT}" ]; then
    log_error "DASMLAB_GHCR_PAT environment variable is required"
    log_info "Set it via one of:"
    log_info "  1. export DASMLAB_GHCR_PAT=your_token"
    log_info "  2. Create ~/gh-pat file with: export DASMLAB_GHCR_PAT=your_token"
    log_info "  3. Create ~/gh-pat/token file with just the token"
    log_info "  4. Create /Users/dasm/gh_token file with just the token"
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

log_step "Building and pushing Iskoces release images"
log_info "Release tag: ${RELEASE_TAG}"
log_info "This will build and push Iskoces images with the '${RELEASE_TAG}' tag"
log_info "These images will be used by the user install script (install_glooscap.sh --plugins iskoces)"
echo ""

# Check if buildx is available for multi-arch builds
USE_BUILDX=false
if docker buildx version >/dev/null 2>&1; then
    # Check if buildx can build for multiple platforms
    if docker buildx inspect --builder default >/dev/null 2>&1 || docker buildx create --name release-builder --use >/dev/null 2>&1; then
        USE_BUILDX=true
        log_info "Docker buildx detected - will build multi-arch images (linux/arm64,linux/amd64)"
    else
        log_warn "Docker buildx detected but builder setup failed - will build for current architecture only"
    fi
else
    log_warn "Docker buildx not available - will build for current architecture only"
    log_warn "For multi-arch releases, install buildx or run this script on both ARM64 and AMD64 machines"
fi

# Build Iskoces image
log_step "Building Iskoces image"
log_info "Building Iskoces image..."
cd "${PROJECT_ROOT}"

if [ "$USE_BUILDX" = true ]; then
    # Use buildx for multi-arch build
    log_info "Using buildx for multi-arch Iskoces build..."
    if docker buildx build \
        --platform linux/arm64,linux/amd64 \
        --push \
        --tag "${ISKOCES_IMG}" \
        . 2>&1; then
        log_success "Iskoces image built and pushed (multi-arch): ${ISKOCES_IMG}"
    else
        BUILDX_ERROR=$?
        log_error "Failed to build Iskoces image with buildx (exit code: ${BUILDX_ERROR})"
        log_info "Falling back to single-arch build..."
        USE_BUILDX=false
    fi
fi

if [ "$USE_BUILDX" = false ]; then
    # Use buildme.sh to build, then retag, or build directly
    if [ -f "./buildme.sh" ]; then
        log_info "Using buildme.sh to build Iskoces..."
        ./buildme.sh || {
            log_warn "buildme.sh failed, trying direct docker build..."
            # Fallback to direct docker build
            docker build -t iskoces-server:scratch . || {
                log_error "Failed to build Iskoces image"
                exit 1
            }
        }
        
        # Verify the scratch image exists
        if ! docker images --format "{{.Repository}}:{{.Tag}}" | grep -q "^iskoces-server:scratch$"; then
            log_error "Iskoces scratch image not found after build"
            log_info "Available iskoces images:"
            docker images | grep "iskoces-server" || log_warn "No iskoces-server images found"
            exit 1
        fi
        
        # Retag from scratch to released
        log_info "Tagging image as released..."
        docker tag iskoces-server:scratch "${ISKOCES_IMG}" || {
            log_error "Failed to tag Iskoces image from scratch to ${ISKOCES_IMG}"
            log_info "Available images:"
            docker images | grep "iskoces-server"
            exit 1
        }
        
        # Verify the released tag exists
        if ! docker images --format "{{.Repository}}:{{.Tag}}" | grep -q "^${ISKOCES_IMG}$"; then
            log_error "Failed to verify released image tag: ${ISKOCES_IMG}"
            exit 1
        fi
    else
        log_info "buildme.sh not found, using direct docker build..."
        docker build -t "${ISKOCES_IMG}" . || {
            log_error "Failed to build Iskoces image"
            exit 1
        }
    fi
    log_success "Iskoces image built: ${ISKOCES_IMG}"
    
    # Push Iskoces image
    log_info "Pushing Iskoces image to registry..."
    docker push "${ISKOCES_IMG}" || {
        log_error "Failed to push Iskoces image"
        exit 1
    }
    log_success "Iskoces image pushed: ${ISKOCES_IMG}"
fi

# Success summary
log_step "Release images pushed successfully!"
if [ "$USE_BUILDX" = true ]; then
    log_success "Iskoces image has been built and pushed with the '${RELEASE_TAG}' tag (multi-arch: arm64, amd64)"
else
    log_success "Iskoces image has been built and pushed with the '${RELEASE_TAG}' tag (single architecture)"
    log_warn "NOTE: Image was built for current architecture only. For multi-arch support, ensure buildx is available or run on both ARM64 and AMD64 machines."
fi
echo ""
log_info "Released image:"
echo "  - ${ISKOCES_IMG}"
echo ""
log_info "This image is now available for use by:"
echo "  - install_glooscap.sh --plugins iskoces (user installation script)"
echo "  - Any deployment using ISKOCES_VERSION=released"
echo ""
log_info "To verify, you can check the registry:"
echo "  docker pull ${ISKOCES_IMG}"
echo ""
if [ "$USE_BUILDX" = true ]; then
    log_info "To verify multi-arch support:"
    echo "  docker buildx imagetools inspect ${ISKOCES_IMG}"
    echo ""
fi

