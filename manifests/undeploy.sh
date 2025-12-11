#!/usr/bin/env bash
# undeploy.sh
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
MANIFESTS_DIR="${SCRIPT_DIR}"

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

# Optionally delete PVC (models will be lost)
if [[ "${DELETE_PVC:-false}" == "true" ]]; then
    log_warn "Deleting PVC (models will be lost)..."
    kubectl delete -f "${MANIFESTS_DIR}/pvc.yaml" --ignore-not-found=true || true
else
    log_info "PVC preserved (models will be retained)"
    log_info "To delete PVC and models, run: DELETE_PVC=true ./undeploy.sh"
fi

# Delete namespace (this will delete everything in the namespace)
if [[ "${DELETE_NAMESPACE:-false}" == "true" ]]; then
    log_warn "Deleting namespace (all resources will be removed)..."
    kubectl delete -f "${MANIFESTS_DIR}/namespace.yaml" --ignore-not-found=true || true
else
    log_info "Namespace preserved"
    log_info "To delete namespace, run: DELETE_NAMESPACE=true ./undeploy.sh"
fi

log_success "Iskoces removed successfully!"

