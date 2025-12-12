#!/usr/bin/env bash
# cycle-test.sh
# Runs the complete dev cycle for Iskoces: build -> deploy -> undeploy
# Reports results for each step

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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
    echo -e "${BLUE}STEP $1: $2${NC}"
    echo "=========================================="
    echo ""
}

# Results tracking
RESULTS=()
STEP=0

run_step() {
    STEP=$((STEP + 1))
    STEP_NAME="$1"
    SCRIPT="$2"
    
    log_step "${STEP}" "${STEP_NAME}"
    
    if [ -f "${SCRIPT}" ]; then
        if bash "${SCRIPT}" 2>&1; then
            RESULTS+=("${STEP}. ${STEP_NAME}: ✓ SUCCESS")
            log_success "${STEP_NAME} completed successfully"
            return 0
        else
            RESULTS+=("${STEP}. ${STEP_NAME}: ✗ FAILED")
            log_error "${STEP_NAME} failed"
            return 1
        fi
    else
        log_error "Script not found: ${SCRIPT}"
        RESULTS+=("${STEP}. ${STEP_NAME}: ✗ SCRIPT NOT FOUND")
        return 1
    fi
}

# Check for registry credentials
if [ -z "${DASMLAB_GHCR_PAT:-}" ]; then
    log_warn "DASMLAB_GHCR_PAT not set - image build/push will fail"
    log_info "Set it with: export DASMLAB_GHCR_PAT=your_token"
    log_info "The token should be a GitHub PAT with 'write:packages' permission"
fi

# Check if cluster is accessible
if ! kubectl cluster-info &> /dev/null 2>&1; then
    log_error "Cannot connect to Kubernetes cluster"
    log_info "Please ensure Kubernetes cluster is running"
    log_info "For Glooscap setup, run: cd /path/to/glooscap/infra/macos-foss && ./install_glooscap.sh"
    exit 1
fi

# Start
echo ""
log_info "Starting Iskoces dev cycle test..."
echo "This will run 3 steps:"
echo "  1. Build and push image"
echo "  2. Deploy Iskoces"
echo "  3. Undeploy Iskoces"
echo ""

# Step 1: Build and push
if [ -n "${DASMLAB_GHCR_PAT:-}" ]; then
    run_step "Build and Push Image" "${SCRIPT_DIR}/build-and-load-images.sh"
    BUILD_RESULT=$?
else
    log_warn "Skipping build (DASMLAB_GHCR_PAT not set)"
    RESULTS+=("1. Build and Push Image: ⊘ SKIPPED (DASMLAB_GHCR_PAT not set)")
    BUILD_RESULT=0  # Not a failure, just skipped
fi

# Step 2: Deploy
if [ ${BUILD_RESULT} -eq 0 ]; then
    run_step "Deploy Iskoces" "${SCRIPT_DIR}/deploy-iskoces.sh"
    DEPLOY_RESULT=$?
else
    log_warn "Skipping deploy (build failed or skipped)"
    RESULTS+=("2. Deploy Iskoces: ⊘ SKIPPED (build failed or skipped)")
    DEPLOY_RESULT=1
fi

# Step 3: Undeploy
if [ ${DEPLOY_RESULT} -eq 0 ]; then
    run_step "Undeploy Iskoces" "${SCRIPT_DIR}/undeploy-iskoces.sh"
    UNDEPLOY_RESULT=$?
else
    log_warn "Skipping undeploy (deploy failed or skipped)"
    RESULTS+=("3. Undeploy Iskoces: ⊘ SKIPPED")
    UNDEPLOY_RESULT=0
fi

# Final Report
echo ""
echo "=========================================="
echo -e "${BLUE}FINAL REPORT${NC}"
echo "=========================================="
echo ""

for result in "${RESULTS[@]}"; do
    if [[ "${result}" == *"✓ SUCCESS"* ]]; then
        echo -e "${GREEN}${result}${NC}"
    elif [[ "${result}" == *"✗"* ]]; then
        echo -e "${RED}${result}${NC}"
    else
        echo -e "${YELLOW}${result}${NC}"
    fi
done

echo ""
TOTAL_STEPS=${#RESULTS[@]}
SUCCESS_COUNT=$(printf '%s\n' "${RESULTS[@]}" | grep -c "✓ SUCCESS" || echo "0")

if [ "${SUCCESS_COUNT}" -eq "${TOTAL_STEPS}" ]; then
    log_success "All steps completed successfully!"
    exit 0
else
    log_warn "Some steps failed or were skipped"
    log_info "Success: ${SUCCESS_COUNT}/${TOTAL_STEPS} steps"
    exit 1
fi

