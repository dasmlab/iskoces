#!/usr/bin/env bash
set -euo pipefail

# cycleme.sh - Build and push Iskoces server image
# This script builds and pushes the image in one go

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "[cycleme] Building iskoces-server..."
./buildme.sh

echo "[cycleme] Pushing iskoces-server..."
./pushme.sh

echo "[cycleme] ✅ Done!"

