#!/usr/bin/env bash
set -euo pipefail

# testme.sh - Test Iskoces translation service locally
# Usage: ./testme.sh [source_lang] [target_lang] [text_file]
#
# Examples:
#   ./testme.sh en fr testdata/starwars_opening.txt
#   ./testme.sh en es testdata/starwars_opening.txt
#   ./testme.sh fr en testdata/starwars_opening.txt

# Default values
SOURCE_LANG="${1:-en}"
TARGET_LANG="${2:-fr}"
TEXT_FILE="${3:-testdata/starwars_opening.txt}"

# Server address
SERVER_ADDR="${ISKOCES_SERVER_ADDR:-localhost:50051}"

echo "[testme] Testing Iskoces translation service"
echo "[testme] Source language: $SOURCE_LANG"
echo "[testme] Target language: $TARGET_LANG"
echo "[testme] Text file: $TEXT_FILE"
echo "[testme] Server: $SERVER_ADDR"
echo ""

# Check if text file exists
if [[ ! -f "$TEXT_FILE" ]]; then
    echo "❌ Error: Text file not found: $TEXT_FILE"
    echo ""
    echo "Usage: ./testme.sh [source_lang] [target_lang] [text_file]"
    echo "Example: ./testme.sh en fr testdata/starwars_opening.txt"
    exit 1
fi

# Check if server is running
if ! nc -z localhost 50051 2>/dev/null; then
    echo "⚠️  Warning: Server doesn't appear to be running on $SERVER_ADDR"
    echo "   Make sure Iskoces is running: ./runme.sh or make run"
    echo ""
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Build test client if needed
if [[ ! -f bin/test-client ]]; then
    echo "[testme] Building test client..."
    go build -o bin/test-client ./cmd/testclient
fi

# Run the test
echo "[testme] Running translation test..."
echo ""
bin/test-client \
    -addr "$SERVER_ADDR" \
    -source "$SOURCE_LANG" \
    -target "$TARGET_LANG" \
    -file "$TEXT_FILE"

echo ""
echo "[testme] ✅ Test complete!"

