#!/usr/bin/env bash
set -euo pipefail

# runme.sh - Run Iskoces server locally in Docker
# This script runs a locally built instance of iskoces-server

app=iskoces-server
version=scratch

# Volume for persisting LibreTranslate models
MODEL_VOLUME="${app}-models"

echo "[runme] Running ${app}:${version}..."
echo "[runme] gRPC server will be available on port 50051"
echo "[runme] MT engine (LibreTranslate) will be available on port 5000"
echo "[runme] Models will be persisted in volume: ${MODEL_VOLUME}"

# Create volume if it doesn't exist
if ! docker volume inspect "$MODEL_VOLUME" >/dev/null 2>&1; then
    echo "[runme] Creating volume for models: ${MODEL_VOLUME}"
    docker volume create "$MODEL_VOLUME"
fi

# Run the container in detached mode
# -d runs in detached mode (background)
# -p 50051:50051 maps host port 50051 to container port 50051 (gRPC server)
# -p 5000:5000 maps host port 5000 to container port 5000 (MT engine)
# -v mounts the models volume for persistence (LibreTranslate stores in $HOME/.local/share/argos-translate)
# --name sets the container name
CONTAINER_ID=$(docker run -d \
    -p 50051:50051 \
    -p 5000:5000 \
    -v "${MODEL_VOLUME}:/models" \
    -e ISKOCES_MODEL_DIR=/models \
    --name "${app}-instance" \
    "${app}:${version}")

echo "[runme] Container started with ID: ${CONTAINER_ID:0:12}"
echo "[runme] View logs with: docker logs -f ${app}-instance"
echo "[runme] Stop with: docker stop ${app}-instance"
echo "[runme] Remove with: docker rm ${app}-instance"

