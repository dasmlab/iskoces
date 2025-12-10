# Multi-stage build for Iskoces gRPC server with lightweight MT engine
# Stage 1: LibreTranslate/Argos base (Python runtime + MT engine)
FROM python:3.12-slim AS mt-engine

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Install LibreTranslate (default engine)
# Note: For Argos, you would install argostranslate instead
# RUN pip install --no-cache-dir argostranslate
RUN pip install --no-cache-dir libretranslate

# Pre-download models if needed (optional, can be done at runtime)
# LibreTranslate downloads models on first use, but we can pre-download here
# RUN libretranslate --download-models en fr es de it pt

# Stage 2: Go builder
FROM golang:1.23 AS builder

WORKDIR /workspace

# Install protoc compiler and include files
RUN apt-get update && apt-get install -y --no-install-recommends \
    protobuf-compiler \
    libprotobuf-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy proto files
COPY proto/ ./proto/

# Generate proto code (requires protoc and plugins)
# Note: In CI/CD, these should be pre-installed
# Use GOTOOLCHAIN=auto to allow Go to use newer toolchain if needed
RUN GOTOOLCHAIN=auto go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    GOTOOLCHAIN=auto go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate proto stubs
# Include standard protobuf includes for well-known types
RUN mkdir -p pkg/proto/v1 && \
    protoc \
        --go_out=pkg/proto/v1 \
        --go_opt=paths=source_relative \
        --go-grpc_out=pkg/proto/v1 \
        --go-grpc_opt=paths=source_relative \
        --proto_path=proto \
        --proto_path=/usr/include \
        proto/translation.proto

# Copy source code
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /tmp/iskoces-server ./cmd/server

# Stage 3: Final image
FROM debian:12-slim

WORKDIR /app

# Install runtime dependencies including Python and LibreTranslate
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Install LibreTranslate
# Note: --break-system-packages is needed for Debian 12 (PEP 668)
RUN pip3 install --no-cache-dir --break-system-packages libretranslate

# Copy Go binary from builder
COPY --from=builder /tmp/iskoces-server /usr/local/bin/iskoces-server

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Create non-root user (UID will be assigned by OpenShift SCC)
RUN useradd -r -g 0 -u 1001 iskoces || true && \
    chown -R 1001:0 /app && \
    chmod -R g+w /app

# Don't set USER here - let OpenShift SCC handle it
# USER 1001

# Expose ports
# gRPC server
EXPOSE 50051      
# MT engine (if needed for debugging, but should be localhost-only)
EXPOSE 5000       

# Set environment variables
ENV ISKOCES_MT_ENGINE=libretranslate
ENV ISKOCES_MT_URL=http://localhost:5000
ENV ISKOCES_GRPC_PORT=50051
ENV ISKOCES_MT_LANGUAGES=en,fr,es
ENV ISKOCES_MODEL_DIR=/models

ENTRYPOINT ["/entrypoint.sh"]

