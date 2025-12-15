#!/usr/bin/env bash
set -euo pipefail

# run-tests.sh - Comprehensive test suite for Iskoces
# This script runs multiple test scenarios to verify Iskoces functionality

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Server address
SERVER_ADDR="${ISKOCES_SERVER_ADDR:-localhost:50051}"

# Test results
PASSED=0
FAILED=0
TOTAL=0

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((PASSED++))
    ((TOTAL++))
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((FAILED++))
    ((TOTAL++))
}

log_test() {
    echo -e "${YELLOW}[TEST]${NC} $1"
}

# Check if server is running
check_server() {
    log_info "Checking if Iskoces server is running on $SERVER_ADDR..."
    if nc -z localhost 50051 2>/dev/null; then
        log_success "Server is running"
        return 0
    else
        log_error "Server is not running on $SERVER_ADDR"
        echo ""
        echo "Please start the server first:"
        echo "  Local: ./runme.sh or make run"
        echo "  Docker: docker run -p 50051:50051 iskoces-server:scratch"
        echo "  OpenShift: oc port-forward svc/iskoces-service 50051:50051 -n iskoces"
        return 1
    fi
}

# Build test client if needed
build_test_client() {
    if [[ ! -f bin/test-client ]]; then
        log_info "Building test client..."
        if ! make build-test; then
            log_error "Failed to build test client"
            return 1
        fi
    fi
    log_success "Test client ready"
    return 0
}

# Run a single test
run_test() {
    local test_name="$1"
    local source_lang="$2"
    local target_lang="$3"
    local test_file="$4"
    
    log_test "Running: $test_name ($source_lang -> $target_lang)"
    
    if [[ ! -f "$test_file" ]]; then
        log_error "Test file not found: $test_file"
        return 1
    fi
    
    if bin/test-client \
        -addr "$SERVER_ADDR" \
        -source "$source_lang" \
        -target "$target_lang" \
        -file "$test_file" > /tmp/test_output.log 2>&1; then
        log_success "$test_name"
        return 0
    else
        log_error "$test_name (check /tmp/test_output.log for details)"
        return 1
    fi
}

# Main test execution
main() {
    echo "=========================================="
    echo "Iskoces Test Suite"
    echo "=========================================="
    echo ""
    
    # Pre-flight checks
    if ! check_server; then
        exit 1
    fi
    
    if ! build_test_client; then
        exit 1
    fi
    
    echo ""
    log_info "Starting test suite..."
    echo ""
    
    # Test 1: Star Wars opening (English -> French)
    run_test "Star Wars Opening (EN->FR)" "en" "fr" "testdata/starwars_opening.txt"
    
    # Test 2: Star Wars opening (English -> Spanish)
    run_test "Star Wars Opening (EN->ES)" "en" "es" "testdata/starwars_opening.txt"
    
    # Test 3: Technical document (English -> French)
    run_test "Technical Document (EN->FR)" "en" "fr" "testdata/technical_document.txt"
    
    # Test 4: Business email (English -> French)
    run_test "Business Email (EN->FR)" "en" "fr" "testdata/business_email.txt"
    
    # Test 5: Short phrases (English -> French)
    run_test "Short Phrases (EN->FR)" "en" "fr" "testdata/short_phrases.txt"
    
    # Test 6: Literary excerpt (English -> French)
    run_test "Literary Excerpt (EN->FR)" "en" "fr" "testdata/literary_excerpt.txt"
    
    # Test 7: French -> English (reverse translation)
    run_test "French to English (FR->EN)" "fr" "en" "testdata/starwars_opening.txt"
    
    # Test 8: Spanish -> English
    run_test "Spanish to English (ES->EN)" "es" "en" "testdata/starwars_opening.txt"
    
    # Summary
    echo ""
    echo "=========================================="
    echo "Test Summary"
    echo "=========================================="
    echo -e "Total Tests: ${TOTAL}"
    echo -e "${GREEN}Passed: ${PASSED}${NC}"
    echo -e "${RED}Failed: ${FAILED}${NC}"
    echo ""
    
    if [[ $FAILED -eq 0 ]]; then
        log_success "All tests passed!"
        exit 0
    else
        log_error "Some tests failed. Check logs above for details."
        exit 1
    fi
}

# Run main function
main

