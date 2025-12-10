# Iskoces - Lightweight Machine Translation Service

## Overview

Iskoces is a lightweight, self-hosted machine translation service designed to be a drop-in replacement for the heavier Nanabush vLLM implementation. It reuses the same gRPC interface so that Glooscap (the frontend) can seamlessly switch between the two implementations without code changes.

## Architecture

### High-Level Design

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
│  (implements TranslationService proto)      │
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

### Key Design Decisions

1. **Same gRPC Interface**: Reuse the exact same proto definitions from nanabush to ensure compatibility with Glooscap
2. **Translator Abstraction**: Clean Go interface that allows switching between backends
3. **Localhost-Only**: MT engines run in the same container, only accessible via localhost
4. **No Egress**: Container can be locked down with no outbound network access
5. **Go-First**: Pure Go implementation with HTTP calls to local MT engines (no cgo initially)

## Project Structure

```
iskoces/
├── proto/                          # Proto definitions (copied/symlinked from nanabush)
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
│       └── argos.go                # Argos implementation
├── Dockerfile                      # Multi-stage build
├── entrypoint.sh                   # Starts both MT engine and Go server
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Implementation Plan

### Phase 1: Foundation

1. **Setup Go Module**
   - Initialize `go.mod` with appropriate module path
   - Add dependencies: gRPC, protobuf, HTTP client

2. **Proto Setup**
   - Copy or reference proto files from nanabush
   - Generate Go code from proto definitions
   - Ensure compatibility with existing proto package structure

3. **Project Structure**
   - Create directory structure
   - Set up Makefile for building, testing, proto generation

### Phase 2: Translator Interface & Backends

1. **Translator Interface**
   ```go
   type Translator interface {
       Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error)
       CheckHealth(ctx context.Context) error
       SupportedLanguages() ([]string, error)
   }
   ```

2. **LibreTranslate Backend**
   - HTTP client to `http://127.0.0.1:5000`
   - Implement `/translate` endpoint
   - Handle language code mapping (e.g., "EN" -> "en", "fr-CA" -> "fr")
   - Health check via `/languages` or similar

3. **Argos Backend**
   - HTTP client to `http://127.0.0.1:5000` (or different port)
   - Implement Argos-specific API
   - Handle language code mapping
   - Health check

4. **Backend Factory**
   - Configuration-based selection (env var or config file)
   - Factory function to create appropriate backend

### Phase 3: gRPC Service Implementation

1. **TranslationService Implementation**
   - Implement all gRPC methods from proto:
     - `RegisterClient`
     - `Heartbeat`
     - `CheckTitle`
     - `Translate`
     - `TranslateStream` (optional, can be simplified)
   - Use Translator interface for actual translation
   - Handle language code conversion (proto uses "EN"/"fr-CA", backends may use "en"/"fr")

2. **Language Code Mapping**
   - Utility to convert between proto language codes and backend codes
   - Handle BCP 47 tags (e.g., "fr-CA" -> "fr")

3. **Error Handling**
   - Graceful degradation
   - Proper gRPC error codes
   - Logging

### Phase 4: Containerization

1. **Dockerfile (Multi-Stage)**
   ```dockerfile
   # Stage 1: LibreTranslate/Argos base
   FROM python:3.12-slim AS mt-engine
   RUN pip install libretranslate  # or argostranslate
   # Pre-download models if needed
   
   # Stage 2: Go builder
   FROM golang:1.23 AS builder
   # Build Go binary
   
   # Stage 3: Final image
   FROM debian:12-slim
   # Copy Go binary
   # Copy Python runtime and MT engine
   # Copy entrypoint script
   ```

2. **Entrypoint Script**
   - Start MT engine in background (localhost only)
   - Wait for health check
   - Start Go gRPC server
   - Handle signals gracefully

3. **Configuration**
   - Environment variables for:
     - MT engine selection (LIBRETRANSLATE or ARGOS)
     - MT engine port
     - gRPC server port
     - Language model selection (if applicable)

### Phase 5: Testing & Integration

1. **Unit Tests**
   - Translator interface implementations
   - Language code mapping
   - Error handling

2. **Integration Tests**
   - Test with actual LibreTranslate/Argos instances
   - Test gRPC service end-to-end

3. **Glooscap Integration**
   - Verify compatibility with existing Glooscap client
   - Test all gRPC methods
   - Verify heartbeat and registration flow

## Configuration

### Environment Variables

- `ISKOCES_MT_ENGINE`: `libretranslate` or `argos` (default: `libretranslate`)
- `ISKOCES_MT_PORT`: Port for MT engine (default: `5000`)
- `ISKOCES_MT_HOST`: Host for MT engine (default: `127.0.0.1`)
- `ISKOCES_GRPC_PORT`: gRPC server port (default: `50051`)
- `ISKOCES_GRPC_INSECURE`: Run without TLS (default: `true`)

### Language Code Handling

Proto uses:
- Source: `"EN"`, `"FR"`, etc. (uppercase)
- Target: `"fr-CA"`, `"en-US"` (BCP 47)

Backends typically use:
- LibreTranslate: `"en"`, `"fr"` (lowercase ISO 639-1)
- Argos: Similar

We'll need mapping utilities to convert between formats.

## Deployment Considerations

1. **Resource Requirements**
   - Much lighter than vLLM (no GPU needed)
   - CPU-only inference
   - Lower memory footprint

2. **Network Isolation**
   - No egress required
   - All communication via localhost
   - Can use NetworkPolicy to block all outbound traffic

3. **Model Storage**
   - Models can be baked into image
   - Or mounted from ConfigMap/Secret
   - Or downloaded on first run (if egress allowed initially)

4. **Scaling**
   - Stateless service (except client registration)
   - Can run multiple replicas
   - Load balancing via Kubernetes Service

## Future Enhancements

1. **cgo Integration** (if needed)
   - Direct C library integration for better performance
   - No HTTP overhead

2. **Model Selection**
   - Support multiple models per backend
   - Model selection based on language pair

3. **Caching**
   - Cache translations for repeated content
   - Redis or in-memory cache

4. **Metrics & Observability**
   - Prometheus metrics
   - OTEL tracing
   - Request/response logging

## Dependencies

### Go Dependencies
- `google.golang.org/grpc`
- `google.golang.org/protobuf`
- `net/http` (for MT engine clients)

### Python Dependencies (in container)
- `libretranslate` or `argostranslate`
- Model files (downloaded automatically or pre-baked)

## Next Steps

1. Review and approve this plan
2. Set up project structure
3. Implement Translator interface and LibreTranslate backend first
4. Implement gRPC service
5. Create Dockerfile and entrypoint
6. Test with Glooscap
7. Add Argos backend support
8. Documentation and deployment guides

