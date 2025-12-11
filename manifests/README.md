# Iskoces Kubernetes Manifests

This directory contains Kubernetes manifests for deploying Iskoces (lightweight machine translation service) to a Kubernetes cluster.

## Directory Structure

```
manifests/
├── namespace.yaml      # Namespace definition
├── configmap.yaml     # Configuration (MT engine, languages, etc.)
├── pvc.yaml          # PersistentVolumeClaim for model storage
├── deployment.yaml    # Deployment for Iskoces server
└── service.yaml       # Service for gRPC and HTTP endpoints
```

## Deployment Order

Manifests should be applied in this order:

1. **Namespace**: Creates the `iskoces` namespace
2. **ConfigMap**: Configuration for Iskoces (MT engine, languages, etc.)
3. **PVC**: Persistent storage for translation models
4. **Deployment**: Deploys the Iskoces server
5. **Service**: Exposes gRPC (50051) and HTTP (5000) endpoints

## Configuration

### ConfigMap

The `configmap.yaml` defines environment variables for Iskoces:

- `ISKOCES_MT_ENGINE`: Translation engine (`libretranslate` or `argos`)
- `ISKOCES_MT_LANGUAGES`: Comma-separated language codes (e.g., `en,fr,es`) or `all`
- `ISKOCES_LOG_LEVEL`: Log level (`debug`, `info`, `warn`, `error`)

### Model Storage

Models are stored in a PersistentVolumeClaim (`iskoces-models`) mounted at `/models`. This ensures models persist across pod restarts.

For local development (k3d/minikube), the PVC uses `storageClassName: local-path`.

## Image Configuration

For local development, you may need to:

1. **Build image locally** using Docker:
   ```bash
   cd /path/to/iskoces
   docker build -t ghcr.io/dasmlab/iskoces-server:latest .
   ```

2. **Load image into k3d**:
   ```bash
   # k3d uses Docker directly, so images are automatically available
   # Images built with Docker are accessible to k3d
   # Or import explicitly if needed:
   # k3d image import ghcr.io/dasmlab/iskoces-server:latest -c glooscap
   ```

3. **Update imagePullPolicy** in `deployment.yaml`:
   - Change `imagePullPolicy: IfNotPresent` to `imagePullPolicy: Never` for local-only images

## Integration with Glooscap

To use Iskoces with Glooscap:

1. Deploy Iskoces to the cluster
2. Configure Glooscap operator to use Iskoces:
   ```yaml
   env:
   - name: TRANSLATION_SERVICE_ADDR
     value: "iskoces-service.iskoces.svc:50051"
   - name: TRANSLATION_SERVICE_TYPE
     value: "iskoces"
   - name: TRANSLATION_SERVICE_SECURE
     value: "false"
   ```

Or configure via Glooscap UI (Settings → Translation Service).

## Troubleshooting

### Pod not starting

- Check logs: `kubectl logs -n iskoces deployment/iskoces-server`
- Check if models are downloading (first startup can take time)
- Verify PVC is bound: `kubectl get pvc -n iskoces`

### Models not persisting

- Verify PVC is mounted: `kubectl describe pod -n iskoces -l app.kubernetes.io/name=iskoces`
- Check PVC status: `kubectl get pvc -n iskoces`

### gRPC connection issues

- Verify service exists: `kubectl get svc -n iskoces`
- Test from within cluster: `kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- grpc_health_probe -addr=iskoces-service.iskoces.svc:50051`

### Resource limits

If models fail to load, you may need to increase memory limits in `deployment.yaml`:
```yaml
resources:
  limits:
    memory: 8Gi  # Increase if needed
```

