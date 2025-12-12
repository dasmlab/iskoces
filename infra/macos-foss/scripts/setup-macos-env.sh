#!/usr/bin/env bash
# setup-macos-env.sh
# Sets up macOS environment for Iskoces development
# Installs build dependencies (Xcode Command Line Tools, Go, etc.)

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

check_command() {
    if command -v "$1" &> /dev/null; then
        return 0
    else
        return 1
    fi
}

# Check if running on macOS
if [[ "$(uname)" != "Darwin" ]]; then
    log_error "This script is designed for macOS only"
    exit 1
fi

log_info "Starting macOS environment setup for Iskoces development..."

# Check for Homebrew
if ! check_command brew; then
    log_warn "Homebrew not found. Installing Homebrew..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    
    # Setup Homebrew shellenv for Apple Silicon or Intel
    if [[ -d "/opt/homebrew" ]]; then
        # Apple Silicon
        BREW_PREFIX="/opt/homebrew"
    else
        # Intel
        BREW_PREFIX="/usr/local"
    fi
    
    log_info "Setting up Homebrew shell environment..."
    if [[ -f "${HOME}/.zprofile" ]]; then
        if ! grep -q "brew shellenv" "${HOME}/.zprofile"; then
            echo "" >> "${HOME}/.zprofile"
            echo 'eval "$('"${BREW_PREFIX}"'/bin/brew shellenv)"' >> "${HOME}/.zprofile"
        fi
    else
        echo 'eval "$('"${BREW_PREFIX}"'/bin/brew shellenv)"' > "${HOME}/.zprofile"
    fi
    
    # Also add to .zshrc if it exists
    if [[ -f "${HOME}/.zshrc" ]]; then
        if ! grep -q "brew shellenv" "${HOME}/.zshrc"; then
            echo "" >> "${HOME}/.zshrc"
            echo 'eval "$('"${BREW_PREFIX}"'/bin/brew shellenv)"' >> "${HOME}/.zshrc"
        fi
    fi
    
    # Evaluate shellenv for current session
    eval "$(${BREW_PREFIX}/bin/brew shellenv)"
    
    log_success "Homebrew installed and configured"
else
    log_info "Homebrew already installed: $(brew --version | head -n1)"
fi

# Update Homebrew
log_info "Updating Homebrew..."
brew update

# Install Xcode Command Line Tools (provides cc, gcc, clang, make, etc.)
log_info "Checking for Xcode Command Line Tools..."
if ! xcode-select -p &> /dev/null; then
    log_info "Installing Xcode Command Line Tools (this may take a while)..."
    xcode-select --install || {
        log_error "Failed to install Xcode Command Line Tools"
        log_info "Please install manually: xcode-select --install"
        exit 1
    }
    log_success "Xcode Command Line Tools installation started"
    log_warn "Please complete the installation in the popup window, then run this script again"
    exit 0
else
    log_success "Xcode Command Line Tools already installed"
fi

# Verify cc/gcc/clang are available
log_info "Verifying C compiler availability..."
if check_command cc || check_command gcc || check_command clang; then
    CC_CMD=$(command -v cc 2>/dev/null || command -v gcc 2>/dev/null || command -v clang 2>/dev/null)
    log_success "C compiler found: ${CC_CMD}"
    log_info "Compiler version: $(${CC_CMD} --version 2>/dev/null | head -n1 || echo 'version check failed')"
else
    log_error "No C compiler found (cc, gcc, or clang)"
    log_info "Please ensure Xcode Command Line Tools are properly installed"
    exit 1
fi

# Install Go (if not already installed)
if ! check_command go; then
    log_info "Installing Go..."
    brew install go
    log_success "Go installed"
else
    log_success "Go already installed: $(go version | head -n1)"
fi

# Install protobuf compiler (for proto generation)
if ! check_command protoc; then
    log_info "Installing protobuf compiler..."
    brew install protobuf
    log_success "protoc installed"
else
    log_success "protoc already installed: $(protoc --version 2>/dev/null | head -n1 || echo 'installed')"
fi

# Install make (usually comes with Xcode CLT, but verify)
if ! check_command make; then
    log_warn "make not found, installing..."
    brew install make
    log_success "make installed"
else
    log_success "make already installed: $(make --version 2>/dev/null | head -n1 || echo 'installed')"
fi

# Summary
echo ""
log_success "macOS environment setup complete!"
echo ""
log_info "Installed/verified tools:"
log_success "  ✓ C compiler (cc/gcc/clang): $(command -v cc 2>/dev/null || command -v gcc 2>/dev/null || command -v clang 2>/dev/null || echo 'not found')"
log_success "  ✓ Go: $(go version 2>/dev/null | head -n1 || echo 'installed')"
log_success "  ✓ protoc: $(protoc --version 2>/dev/null | head -n1 || echo 'installed')"
log_success "  ✓ make: $(make --version 2>/dev/null | head -n1 || echo 'installed')"
echo ""

