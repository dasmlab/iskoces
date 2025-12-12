#!/usr/bin/env bash
# deploy-iskoces.sh
# Deploys Iskoces to the Kubernetes cluster

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

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    log_error "kubectl not found. Please run setup first"
    exit 1
fi

# Check if cluster is accessible
if ! kubectl cluster-info &> /dev/null; then
    log_error "Cannot connect to Kubernetes cluster"
    log_info "Please ensure Kubernetes cluster is running"
    exit 1
fi

log_info "Deploying Iskoces to Kubernetes cluster..."

# Create namespace
log_info "Creating namespace..."
kubectl apply -f "${MANIFESTS_DIR}/namespace.yaml"

# Apply ConfigMap
log_info "Applying configuration..."
kubectl apply -f "${MANIFESTS_DIR}/configmap.yaml"

# Apply PVC
log_info "Creating persistent volume for models..."
kubectl apply -f "${MANIFESTS_DIR}/pvc.yaml"

# Wait for PVC to be bound (may take a moment)
log_info "Waiting for PVC to be bound..."
kubectl wait --for=condition=Bound --timeout=60s pvc/iskoces-models -n iskoces || {
    log_warn "PVC may not be bound yet (this is OK for local development)"
}

# Apply Deployment
log_info "Deploying Iskoces server..."
kubectl apply -f "${MANIFESTS_DIR}/deployment.yaml"

# Apply Service
log_info "Creating service..."
kubectl apply -f "${MANIFESTS_DIR}/service.yaml"

# Wait for deployment to be ready
log_info "Waiting for Iskoces to be ready (this may take a few minutes for models to load)..."
kubectl wait --for=condition=available --timeout=600s deployment/iskoces-server -n iskoces || {
    log_warn "Iskoces deployment may not be ready yet"
    log_info "Check status with: kubectl get pods -n iskoces"
    log_info "Check logs with: kubectl logs -n iskoces deployment/iskoces-server"
}

# Show status
echo ""
log_success "Iskoces deployed successfully!"
echo ""
log_info "Deployment status:"
kubectl get pods -n iskoces
echo ""
log_info "Service:"
kubectl get svc -n iskoces
echo ""

# Show service address
SERVICE_ADDR="iskoces-service.iskoces.svc:50051"
log_info "Iskoces gRPC service address: ${SERVICE_ADDR}"
echo ""
log_info "To use with Glooscap, configure in UI:"
echo "  Address: ${SERVICE_ADDR}"
echo "  Type: iskoces"
echo "  Secure: false"
echo ""
log_info "To view logs:"
echo "  kubectl logs -f -n iskoces deployment/iskoces-server"
echo ""

