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

# Install runtime dependencies including Python and build tools
# Build tools (gcc, g++, make) are needed for compiling Python C extensions in LibreTranslate
# Additional dependencies for PyMuPDF (mupdf) compilation on ARM64
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    python3 \
    python3-pip \
    build-essential \
    gcc \
    g++ \
    make \
    cmake \
    pkg-config \
    libfreetype6-dev \
    libjpeg62-turbo-dev \
    libopenjp2-7-dev \
    libtiff5-dev \
    libxcb1-dev \
    && rm -rf /var/lib/apt/lists/*

# Install PyMuPDF first with a version that has ARM64 wheels or better build support
# This prevents LibreTranslate from pulling a version that fails to build on ARM64
# Version 1.25.2+ has better ARM64 support, but we'll let pip find the best available
RUN pip3 install --no-cache-dir --break-system-packages \
    "PyMuPDF>=1.25.2" || \
    pip3 install --no-cache-dir --break-system-packages PyMuPDF

# Install LibreTranslate
# Note: --break-system-packages is needed for Debian 12 (PEP 668)
# Build tools are required here for compiling Python C extensions
# PyMuPDF is already installed above to avoid build issues
# LibreTranslate is large (~3-4GB) because it includes PyTorch and ML models
# Consider using argostranslate for a lighter alternative if size is critical
RUN pip3 install --no-cache-dir --break-system-packages libretranslate && \
    # Clean up pip cache to reduce size
    rm -rf /root/.cache/pip && \
    # Remove unnecessary Python packages if possible
    find /usr/local/lib/python3.12 -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true && \
    find /usr/local/lib/python3.12 -type f -name "*.pyc" -delete 2>/dev/null || true

# Remove build tools to reduce image size (they're only needed during pip install)
# This significantly reduces the final image size (removes ~1-2GB)
RUN apt-get purge -y --auto-remove \
    build-essential \
    gcc \
    g++ \
    make \
    cmake \
    pkg-config \
    libfreetype6-dev \
    libjpeg62-turbo-dev \
    libopenjp2-7-dev \
    libtiff5-dev \
    libxcb1-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy Go binary from builder
COPY --from=builder /tmp/iskoces-server /usr/local/bin/iskoces-server

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Make directories writable by group (OpenShift will assign UID, but group will be 0/root)
# This ensures the container works regardless of what UID OpenShift assigns
RUN chown -R root:0 /app /tmp && \
    chmod -R g+w /app /tmp && \
    chmod -R g+w /usr/local/bin/iskoces-server || true

# Don't create a specific user - let OpenShift SCC assign UID automatically
# OpenShift will run as a non-root UID from the namespace's allocated range
# Files are group-writable (g+w) so any UID in group 0 can write

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

