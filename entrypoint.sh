#!/usr/bin/env bash
set -euo pipefail

# Entrypoint script for Iskoces container
# Starts the MT engine in the background, then starts the Go gRPC server

# Configuration from environment variables
MT_ENGINE="${ISKOCES_MT_ENGINE:-libretranslate}"
MT_URL="${ISKOCES_MT_URL:-http://localhost:5000}"
MT_PORT="${ISKOCES_MT_PORT:-5000}"
GRPC_PORT="${ISKOCES_GRPC_PORT:-50051}"
LOG_LEVEL="${ISKOCES_LOG_LEVEL:-info}"

# Language configuration - comma-separated list of language codes or "all"
# Default: en, fr, es (English, French, Spanish)
# Special: "all" downloads all available models
MT_LANGUAGES="${ISKOCES_MT_LANGUAGES:-en,fr,es}"

# Model storage directory (should be mounted as volume for persistence)
# LibreTranslate stores models in $HOME/.local/share/argos-translate
MODEL_DIR="${ISKOCES_MODEL_DIR:-/models}"

echo "[iskoces] Starting Iskoces container..."
echo "[iskoces] MT Engine: $MT_ENGINE"
echo "[iskoces] MT URL: $MT_URL"
echo "[iskoces] MT Port: $MT_PORT"
echo "[iskoces] gRPC Port: $GRPC_PORT"
echo "[iskoces] Languages: $MT_LANGUAGES"
echo "[iskoces] Model Directory: $MODEL_DIR"

# Function to check if a service is ready
wait_for_service() {
    local url=$1
    local max_attempts=30
    local attempt=0

    echo "[iskoces] Waiting for MT engine at $url..."
    while [ $attempt -lt $max_attempts ]; do
        if curl -s -f "$url/languages" > /dev/null 2>&1 || \
           curl -s -f "$url/health" > /dev/null 2>&1; then
            echo "[iskoces] MT engine is ready!"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 2
    done

    echo "[iskoces] WARNING: MT engine health check failed after $max_attempts attempts"
    echo "[iskoces] Continuing anyway - server will start but translations may fail"
    return 1
}

# Start MT engine based on configuration
case "$MT_ENGINE" in
    libretranslate)
        echo "[iskoces] Starting LibreTranslate on 0.0.0.0:$MT_PORT..."
        # Start LibreTranslate in background, binding to 0.0.0.0 to listen on all interfaces
        # (The Go server will still connect via 127.0.0.1, but LibreTranslate needs to bind to 0.0.0.0)
        # Try both 'libretranslate' command and 'python3 -m libretranslate'
        # Use nohup to prevent it from being killed when parent exits
        # Redirect output to log file for tailing
        
        # Use the libretranslate command (should be in /usr/local/bin/libretranslate)
        if command -v libretranslate >/dev/null 2>&1; then
            echo "[iskoces] Starting LibreTranslate via command: $(which libretranslate)"
            
            # Create model directory if it doesn't exist
            mkdir -p "$MODEL_DIR"
            
            # Build LibreTranslate command with language configuration
            LT_CMD="libretranslate --host 0.0.0.0 --port $MT_PORT"
            
            # Configure languages to load using --load-only flag
            # According to LibreTranslate docs: --load-only accepts comma-separated language codes
            # Example: --load-only en,es,de,fr,ru,zh
            if [ "$MT_LANGUAGES" != "all" ]; then
                # Clean up the language list (remove whitespace, ensure proper format)
                LT_LANGS=$(echo "$MT_LANGUAGES" | tr ',' ' ' | xargs | tr ' ' ',')
                echo "[iskoces] Loading only specified languages: $LT_LANGS"
                LT_CMD="$LT_CMD --load-only $LT_LANGS"
            else
                echo "[iskoces] Loading ALL available language models (this may take a while)"
            fi
            
            # Set model storage directory
            # LibreTranslate stores models in $HOME/.local/share/argos-translate
            # We'll set HOME to point to our model directory
            export HOME="$MODEL_DIR"
            # Also try LT_MODEL_DIR if supported
            export LT_MODEL_DIR="$MODEL_DIR" || true
            echo "[iskoces] Models will be stored in: $MODEL_DIR/.local/share/argos-translate"
            
            # Ensure log file exists and is writable
            touch /tmp/libretranslate.log
            chmod 666 /tmp/libretranslate.log
            # Clear any previous content
            > /tmp/libretranslate.log
            
            # Start in background with unbuffered output
            PYTHONUNBUFFERED=1 nohup $LT_CMD > /tmp/libretranslate.log 2>&1 &
            MT_PID=$!
            echo "[iskoces] Started with PID $MT_PID, output going to /tmp/libretranslate.log"
            
            # Immediately check if we can see any output
            sleep 1
            if [ -f /tmp/libretranslate.log ] && [ -s /tmp/libretranslate.log ]; then
                echo "[iskoces] Initial LibreTranslate output:"
                head -10 /tmp/libretranslate.log | sed 's/^/[iskoces] LT: /'
            else
                echo "[iskoces] WARNING: Log file is empty after 1 second"
            fi
        else
            echo "[iskoces] ERROR: libretranslate command not found in PATH"
            echo "[iskoces] PATH: $PATH"
            exit 1
        fi
        
        echo "[iskoces] LibreTranslate started (PID: $MT_PID)"
        
        # Function to monitor and display LibreTranslate startup progress
        monitor_libretranslate_startup() {
            local log_file="/tmp/libretranslate.log"
            local max_wait=60  # Maximum seconds to monitor
            local elapsed=0
            local last_displayed_line=0
            
            echo "[iskoces] Monitoring LibreTranslate startup progress..."
            
            while [ $elapsed -lt $max_wait ] && kill -0 "$MT_PID" 2>/dev/null; do
                if [ -f "$log_file" ]; then
                    # Get current line count
                    local current_lines=$(wc -l < "$log_file" 2>/dev/null || echo "0")
                    
                    # If we have new lines, display them
                    if [ "$current_lines" -gt "$last_displayed_line" ]; then
                        # Show new lines with prefix
                        tail -n +$((last_displayed_line + 1)) "$log_file" 2>/dev/null | while IFS= read -r line; do
                            # Show all non-empty lines (LibreTranslate output can be verbose)
                            if [ -n "$line" ]; then
                                echo "[iskoces] LibreTranslate: $line"
                            fi
                        done
                        last_displayed_line=$current_lines
                    fi
                    
                    # Check if server is ready
                    if tail -n 50 "$log_file" 2>/dev/null | grep -qi "running on\|started\|ready\|listening\|serving\|Running on"; then
                        echo "[iskoces] LibreTranslate appears to be ready based on logs"
                        return 0  # Server is ready
                    fi
                else
                    # Log file doesn't exist yet, show a dot to indicate we're waiting
                    if [ $((elapsed % 4)) -eq 0 ]; then
                        echo -n "."
                    fi
                fi
                sleep 1
                elapsed=$((elapsed + 1))
            done
            echo ""  # New line after dots
            echo "[iskoces] Monitoring timeout reached or process stopped"
        }
        
        # Give LibreTranslate a moment to start writing logs
        sleep 2
        
        # Wait a moment and check if process is still running
        sleep 2
        if ! kill -0 "$MT_PID" 2>/dev/null; then
            echo "[iskoces] ERROR: LibreTranslate process died immediately. Check logs:"
            echo "[iskoces] ========== LibreTranslate Log File =========="
            if [ -f /tmp/libretranslate.log ]; then
                cat /tmp/libretranslate.log || true
            else
                echo "[iskoces] Log file does not exist - process may have failed to start"
                echo "[iskoces] Attempting to run LibreTranslate directly to see error:"
                libretranslate --host 0.0.0.0 --port "$MT_PORT" 2>&1 | head -30 || echo "[iskoces] Command failed"
            fi
            echo "[iskoces] ============================================="
            exit 1
        fi
        echo "[iskoces] LibreTranslate process is running (PID: $MT_PID)"
        
        # Monitor startup progress in background
        monitor_libretranslate_startup &
        MONITOR_PID=$!
        ;;
    argos)
        echo "[iskoces] Starting Argos Translate..."
        # Note: Argos may need a wrapper HTTP service
        # For now, assume it's already running or will be started separately
        echo "[iskoces] Argos Translate should be available at $MT_URL"
        ;;
    *)
        echo "[iskoces] ERROR: Unknown MT engine: $MT_ENGINE"
        exit 1
        ;;
esac

# Wait for MT engine to be ready (with timeout)
if [ "$MT_ENGINE" = "libretranslate" ]; then
    wait_for_service "$MT_URL" || {
        echo "[iskoces] ERROR: MT engine health check failed. LibreTranslate logs:"
        echo "[iskoces] ========== LibreTranslate Log File Contents =========="
        if [ -f /tmp/libretranslate.log ]; then
            if [ -s /tmp/libretranslate.log ]; then
                cat /tmp/libretranslate.log || true
            else
                echo "[iskoces] Log file exists but is EMPTY - LibreTranslate may not be writing output"
            fi
        else
            echo "[iskoces] Log file /tmp/libretranslate.log does not exist!"
        fi
        echo "[iskoces] ======================================================="
        echo "[iskoces] Checking if LibreTranslate process is still running..."
        if kill -0 "$MT_PID" 2>/dev/null; then
            echo "[iskoces] Process is still running (PID: $MT_PID)"
            echo "[iskoces] Checking if process is listening on port $MT_PORT..."
            # Try to see if anything is listening on the port
            if command -v netstat >/dev/null 2>&1; then
                netstat -tlnp 2>/dev/null | grep ":$MT_PORT " || echo "[iskoces] Nothing listening on port $MT_PORT"
            elif command -v ss >/dev/null 2>&1; then
                ss -tlnp 2>/dev/null | grep ":$MT_PORT " || echo "[iskoces] Nothing listening on port $MT_PORT"
            else
                echo "[iskoces] Cannot check port (netstat/ss not available)"
            fi
        else
            echo "[iskoces] Process is NOT running (PID: $MT_PID) - it may have crashed"
        fi
        echo "[iskoces] Continuing anyway, but translations will likely fail..."
    }
    # Stop monitoring once service is ready
    kill "$MONITOR_PID" 2>/dev/null || true
fi

# Start the Go gRPC server
echo "[iskoces] Starting gRPC server on port $GRPC_PORT..."
exec /usr/local/bin/iskoces-server \
    -port "$GRPC_PORT" \
    -insecure \
    -mt-engine "$MT_ENGINE" \
    -mt-url "$MT_URL" \
    -log-level "$LOG_LEVEL"

