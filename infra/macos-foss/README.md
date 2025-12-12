# Iskoces macOS FOSS Setup

This directory contains everything needed to run Iskoces on macOS using the same infrastructure as Glooscap (Podman and k3d).

## Overview

Iskoces is a lightweight machine translation service that can be deployed alongside Glooscap. This setup provides:

- **Architecture-specific builds**: Images built for ARM64/AMD64
- **Registry integration**: Images pushed to `ghcr.io/dasmlab`
- **Kubernetes deployment**: Deployed to the same k3d cluster as Glooscap

## Prerequisites

- macOS (tested on macOS 12+)
- Kubernetes cluster running (from Glooscap setup)
- `DASMLAB_GHCR_PAT` environment variable set (for pushing images)

## Setup Build Environment

Before building Iskoces, ensure build dependencies are installed:

```bash
cd infra/macos-foss
./scripts/setup-macos-env.sh
```

This will install:
- Xcode Command Line Tools (provides `cc`, `gcc`, `clang`)
- Go (if not already installed)
- protobuf compiler (`protoc`)
- `make`

**Note:** If installing via Glooscap's `install_glooscap.sh --plugins iskoces`, the setup script is automatically called before building.

## Quick Start

### Option 1: Install via Glooscap (Recommended)

Install Iskoces as a plugin when installing Glooscap:

```bash
cd /path/to/glooscap/infra/macos-foss
export DASMLAB_GHCR_PAT=your_github_token
./install_glooscap.sh --plugins iskoces
```

### Option 2: Manual Installation

If Glooscap is already installed:

```bash
cd /path/to/iskoces/infra/macos-foss
export DASMLAB_GHCR_PAT=your_github_token

# Build and push image
./scripts/build-and-load-images.sh

# Deploy to cluster
./scripts/deploy-iskoces.sh
```

## Development Cycle Test

Run the complete development cycle:

```bash
cd infra/macos-foss
export DASMLAB_GHCR_PAT=your_github_token
./scripts/cycle-test.sh
```

This will:
1. Build and push image
2. Deploy Iskoces
3. Undeploy Iskoces

## Directory Structure

```
infra/macos-foss/
├── README.md                 # This file
├── manifests/                # Kubernetes manifests
│   ├── namespace.yaml       # Namespace definition
│   ├── configmap.yaml       # Configuration
│   ├── pvc.yaml             # Persistent volume for models
│   ├── deployment.yaml      # Deployment
│   └── service.yaml         # Service (LoadBalancer)
└── scripts/                 # Scripts
    ├── setup-macos-env.sh   # Setup build environment (cc, go, protoc, etc.)
    ├── build-and-load-images.sh  # Build and push image
    ├── deploy-iskoces.sh    # Deploy Iskoces
    ├── undeploy-iskoces.sh  # Remove Iskoces
    └── cycle-test.sh        # Full cycle test
```

## Configuration

The Iskoces service is configured via ConfigMap:

- `ISKOCES_MT_ENGINE`: Translation engine (`libretranslate` or `argos`)
- `ISKOCES_MT_LANGUAGES`: Comma-separated language codes (e.g., `en,fr,es`)
- `ISKOCES_GRPC_PORT`: gRPC server port (default: `50051`)

## Integration with Glooscap

After deploying Iskoces, configure Glooscap to use it:

1. Open Glooscap UI: http://localhost:8080
2. Go to Settings → Translation Service
3. Configure:
   - **Address**: `iskoces-service.iskoces.svc:50051`
   - **Type**: `iskoces`
   - **Secure**: `false`
4. Click "Set Configuration"

## Troubleshooting

### Image pull errors
- Ensure `DASMLAB_GHCR_PAT` is set
- Check registry secret exists: `kubectl get secret -n iskoces dasmlab-ghcr-pull`

### Pods not starting
- Check logs: `kubectl logs -n iskoces deployment/iskoces-server`
- Check PVC status: `kubectl get pvc -n iskoces`
- Models may take time to load (check readiness probe)

### Service not accessible
- Check service: `kubectl get svc -n iskoces`
- Verify LoadBalancer has external IP (k3d assigns automatically)

## Cleanup

To remove Iskoces:

```bash
./scripts/undeploy-iskoces.sh
```

Or if installed via Glooscap:

```bash
cd /path/to/glooscap/infra/macos-foss
./uninstall_glooscap.sh  # This will also clean up plugins
```

