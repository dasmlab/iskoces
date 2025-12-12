#!/usr/bin/env bash
# undeploy-iskoces.sh
# Removes Iskoces from the Kubernetes cluster

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFESTS_DIR="$(cd "${SCRIPT_DIR}/../manifests" && pwd)"

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

log_info "Removing Iskoces from Kubernetes cluster..."

# Delete Service
log_info "Deleting service..."
kubectl delete -f "${MANIFESTS_DIR}/service.yaml" --ignore-not-found=true || true

# Delete Deployment
log_info "Deleting deployment..."
kubectl delete -f "${MANIFESTS_DIR}/deployment.yaml" --ignore-not-found=true || true

# Delete ConfigMap
log_info "Deleting configuration..."
kubectl delete -f "${MANIFESTS_DIR}/configmap.yaml" --ignore-not-found=true || true

# Delete PVC (models will be lost)
log_info "Deleting PVC..."
kubectl delete -f "${MANIFESTS_DIR}/pvc.yaml" --ignore-not-found=true || true

# Delete namespace
log_info "Deleting namespace..."
kubectl delete -f "${MANIFESTS_DIR}/namespace.yaml" --ignore-not-found=true || true

log_success "Iskoces removed successfully!"

