# Iskoces - Lightweight Machine Translation Service

Iskoces is a lightweight, self-hosted machine translation service designed to be a drop-in replacement for the heavier Nanabush vLLM implementation. It reuses the same gRPC interface so that Glooscap (the frontend) can seamlessly switch between the two implementations without code changes.

## Features

- **Same gRPC Interface**: Compatible with existing Glooscap client code
- **Multiple Backends**: Supports LibreTranslate and Argos Translate
- **Self-Hosted**: No external dependencies, runs entirely within the container
- **Lightweight**: No GPU required, CPU-only inference
- **Network Isolation**: All communication via localhost, no egress needed

## Architecture

```
┌─────────────┐
│  Glooscap   │  (Frontend - unchanged)
│  (gRPC      │
│   Client)   │
└──────┬──────┘
       │ gRPC (same proto as nanabush)
       │
┌──────▼─────────────────────────────────────┐
│         Iskoces gRPC Server                │
│  (implements TranslationService proto)     │
│                                             │
│  ┌──────────────────────────────────────┐  │
│  │   Translator Interface (Go)          │  │
│  │                                      │  │
│  │  ┌──────────────┐  ┌──────────────┐ │  │
│  │  │ LibreTranslate│  │   Argos      │ │  │
│  │  │  Backend     │  │   Backend    │ │  │
│  │  │ (HTTP)       │  │  (HTTP)      │ │  │
│  │  └──────────────┘  └──────────────┘ │  │
│  └──────────────────────────────────────┘  │
│                                             │
│  ┌──────────────────────────────────────┐  │
│  │  LibreTranslate/Argos Process        │  │
│  │  (localhost:5000)                     │  │
│  └──────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
```

## Prerequisites

1. **Go 1.21+** installed
2. **protoc compiler** installed (see below)
3. **protoc plugins:**
   - `protoc-gen-go`
   - `protoc-gen-go-grpc`

## Setup

### Install protoc

```bash
# Ubuntu/Debian
sudo apt install protobuf-compiler

# macOS
brew install protobuf

# Verify installation
protoc --version
```

### Install protoc plugins

From the project root:

```bash
make install-protoc
```

Or manually:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### Generate proto stubs

From the project root:

```bash
make proto
```

This will generate:
- `pkg/proto/v1/*.pb.go` - Protocol buffer types
- `pkg/proto/v1/*_grpc.pb.go` - gRPC service stubs

### Build

```bash
make deps   # Download dependencies
make build  # Build binary
```

Binary will be at: `bin/iskoces-server`

## Configuration

### Environment Variables

- `ISKOCES_MT_ENGINE`: Translation engine to use (`libretranslate` or `argos`, default: `libretranslate`)
- `ISKOCES_MT_URL`: Base URL for MT engine API (default: `http://127.0.0.1:5000`)
- `ISKOCES_MT_PORT`: Port for MT engine (default: `5000`)
- `ISKOCES_GRPC_PORT`: gRPC server port (default: `50051`)
- `ISKOCES_LOG_LEVEL`: Log level (`debug`, `info`, `warn`, `error`, default: `info`)

### Command-line Flags

- `-port`: gRPC server port (default: `50051`)
- `-insecure`: Run in insecure mode, no TLS (default: `true`)
- `-mt-engine`: Translation engine (`libretranslate` or `argos`, default: `libretranslate`)
- `-mt-url`: Base URL for MT engine API (default: `http://127.0.0.1:5000`)
- `-log-level`: Log level (`debug`, `info`, `warn`, `error`, default: `info`)

## Helper Scripts

The project includes helper scripts following the organization's conventions:

- **`buildme.sh`** - Build the Docker image with a local "scratch" tag
- **`pushme.sh`** - Tag and push the image to GitHub Container Registry (ghcr.io/dasmlab)
- **`runme.sh`** - Run the locally built Docker container
- **`cycleme.sh`** - Build and push in one go (convenience script)

### Quick Start with Helper Scripts

```bash
# Build the Docker image
./buildme.sh

# Run locally
./runme.sh

# Build and push to registry
./cycleme.sh
```

## Running Locally

### Option 1: Run with Local LibreTranslate

1. Install LibreTranslate:
   ```bash
   pip install libretranslate
   ```

2. Start LibreTranslate:
   ```bash
   libretranslate --host 127.0.0.1 --port 5000
   ```

3. In another terminal, start Iskoces:
   ```bash
   make run
   # Or directly:
   ./bin/iskoces-server -port 50051 -mt-engine libretranslate -mt-url http://127.0.0.1:5000
   ```

### Option 2: Run with Docker

Build and run the container:

```bash
# Build the image
docker build -t iskoces-server:scratch .
# Or use the helper script:
./buildme.sh

# Run the container
docker run -p 50051:50051 iskoces-server:scratch
# Or use the helper script:
./runme.sh
```

The container will:
1. Start LibreTranslate (or Argos) on localhost:5000
2. Wait for it to be ready
3. Start the gRPC server on port 50051

## Testing

### Quick Test with testme.sh

The easiest way to test Iskoces is using the `testme.sh` script with the included Star Wars opening crawl text:

```bash
# Test English to French (default)
./testme.sh

# Test English to Spanish
./testme.sh en es

# Test French to English
./testme.sh fr en

# Use a custom text file
./testme.sh en fr path/to/your/text.txt
```

The script will:
1. Check if the server is running
2. Build the test client if needed
3. Register with the server
4. Translate the text file
5. Display both original and translated text

### Using the Test Client Directly

You can also use the test client directly:

```bash
# Build the test client
make build-test

# Run translation
bin/test-client \
    -addr localhost:50051 \
    -source en \
    -target fr \
    -file testdata/starwars_opening.txt

# Or translate inline text
bin/test-client \
    -addr localhost:50051 \
    -source en \
    -target fr \
    -text "Hello, world!"
```

### Using grpcurl

For more advanced testing with grpcurl:

```bash
# Check health
grpc_health_probe -addr localhost:50051

# Register a client
grpcurl -plaintext -d '{
  "client_name": "test-client",
  "client_version": "1.0.0",
  "namespace": "test"
}' localhost:50051 nanabush.v1.TranslationService/RegisterClient

# Translate a title
grpcurl -plaintext -d '{
  "job_id": "test-123",
  "primitive": "PRIMITIVE_TITLE",
  "title": "Hello World",
  "source_language": "EN",
  "target_language": "fr-CA"
}' localhost:50051 nanabush.v1.TranslationService/Translate
```

## Language Code Handling

Iskoces automatically converts between different language code formats:

- **Proto format**: `"EN"`, `"fr-CA"` (uppercase, BCP 47)
- **Backend format**: `"en"`, `"fr"` (lowercase ISO 639-1)

The `LanguageMapper` handles this conversion automatically.

## Integration with Glooscap

Iskoces is designed to be a drop-in replacement for Nanabush. To use it with Glooscap:

1. Set the `NANABUSH_GRPC_ADDR` environment variable to point to Iskoces:
   ```bash
   export NANABUSH_GRPC_ADDR=iskoces-service.namespace.svc:50051
   ```

2. Glooscap will automatically connect to Iskoces using the same gRPC client code.

## Development

### Project Structure

```
iskoces/
├── proto/                          # Proto definitions
│   └── translation.proto
├── cmd/
│   └── server/
│       └── main.go                # gRPC server entrypoint
├── pkg/
│   ├── proto/
│   │   └── v1/                     # Generated proto code
│   ├── service/
│   │   └── translation_service.go  # gRPC service implementation
│   └── translate/
│       ├── translator.go           # Translator interface
│       ├── libretranslate.go       # LibreTranslate implementation
│       ├── argos.go                # Argos implementation
│       └── factory.go              # Factory for creating translators
├── Dockerfile                      # Multi-stage build
├── entrypoint.sh                   # Container entrypoint
├── Makefile
└── README.md
```

### Adding a New Backend

1. Implement the `Translator` interface in `pkg/translate/`
2. Add the engine type to `EngineType` enum in `factory.go`
3. Update `NewTranslator` to handle the new engine type
4. Update the Dockerfile and entrypoint script if needed

## Logging

Iskoces uses [logrus](https://github.com/sirupsen/logrus) for structured logging. Log levels can be configured via the `-log-level` flag or `ISKOCES_LOG_LEVEL` environment variable.

Example log output:
```
INFO[2025-01-XX...] Starting Iskoces gRPC server    port=50051 insecure=true mt_engine=libretranslate
INFO[2025-01-XX...] Checking translator health...
INFO[2025-01-XX...] Translator health check passed
INFO[2025-01-XX...] gRPC server listening            port=50051
```

## Deployment

### Kubernetes/OpenShift

Iskoces can be deployed as a Kubernetes Deployment. Example:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: iskoces
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: iskoces
        image: iskoces:latest
        ports:
        - containerPort: 50051
        env:
        - name: ISKOCES_MT_ENGINE
          value: "libretranslate"
        - name: ISKOCES_GRPC_PORT
          value: "50051"
```

### Network Policies

Since Iskoces only communicates via localhost, you can lock it down with NetworkPolicy:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: iskoces-no-egress
spec:
  podSelector:
    matchLabels:
      app: iskoces
  policyTypes:
  - Egress
  # No egress rules = no outbound traffic allowed
```

## Differences from Nanabush

- **Backend**: Uses lightweight MT engines (LibreTranslate/Argos) instead of vLLM
- **Resource Requirements**: No GPU needed, lower memory footprint
- **Performance**: Faster startup, but potentially lower translation quality for complex texts
- **Deployment**: Single container with both server and MT engine

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]

