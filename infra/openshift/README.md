# Iskoces OpenShift Deployment

This directory contains OpenShift-specific manifests for deploying Iskoces to an OpenShift cluster.

## Overview

Iskoces is a lightweight machine translation service that can be deployed on OpenShift. This setup provides:

- **Production-ready deployment**: Uses production images from `ghcr.io/dasmlab`
- **OpenShift security**: Proper security contexts and SCC compliance
- **Persistent storage**: Models stored in PVC
- **Service and Route**: Internal service and optional external route

## Prerequisites

- OpenShift cluster (4.x+)
- `dasmlab-ghcr-pull` image pull secret in the `iskoces` namespace
- Appropriate storage class configured (or use default)

## Deployment

### 1. Create Namespace

```bash
oc apply -f namespace.yaml
```

### 2. Create Image Pull Secret

If not already created, create the image pull secret using the helper script:

```bash
DASMLAB_GHCR_PAT=your_token ./create-registry-secret.sh iskoces
```

Or manually:

```bash
oc create secret docker-registry dasmlab-ghcr-pull \
  --docker-server=ghcr.io \
  --docker-username=lmcdasm \
  --docker-password=<your_github_token> \
  --namespace=iskoces
```

**Note**: The helper script (`create-registry-secret.sh`) ensures the namespace exists and makes the operation idempotent.

### 3. Deploy Iskoces

**Important**: Apply manifests in order to ensure namespace is created first:

```bash
# Apply namespace first (required for other resources)
oc apply -f namespace.yaml

# Then apply all other manifests
oc apply -f configmap.yaml
oc apply -f pvc.yaml
oc apply -f deployment.yaml
oc apply -f service.yaml
oc apply -f route.yaml
```

Or apply all at once (namespace will be created first automatically):

```bash
oc apply -f .
```

**Note**: If you see "namespace not found" errors, apply `namespace.yaml` first, then apply the rest.

### 4. Verify Deployment

```bash
# Check pods
oc get pods -n iskoces

# Check service
oc get svc -n iskoces

# Check route
oc get route -n iskoces

# View logs
oc logs -f deployment/iskoces-server -n iskoces
```

## Configuration

### Update Cluster Domain

Edit `route.yaml` and update the `host` field with your cluster domain:

```yaml
spec:
  host: iskoces.apps.<your-cluster-domain>
```

### Update Storage Class

The PVC is configured to use `lvms-vg1` storage class (topolvm.io provisioner) by default. If you need a different storage class, edit `pvc.yaml`:

```yaml
spec:
  storageClassName: lvms-vg1  # or your storage class
```

To check available storage classes in your cluster:
```bash
oc get sc
```

### Update Image Tag

To use a specific image version, edit `deployment.yaml`:

```yaml
containers:
- name: iskoces-server
  image: ghcr.io/dasmlab/iskoces-server:v0.1.0  # or :latest
```

### Configure Languages

Edit `configmap.yaml` to change which language models are loaded:

```yaml
data:
  ISKOCES_MT_LANGUAGES: "en,fr,es,de,it"  # Add more languages
```

## Integration with Glooscap

After deploying Iskoces, configure Glooscap to use it:

1. **Via Glooscap UI:**
   - Go to Settings → Translation Service
   - Set Address: `iskoces-service.iskoces.svc:50051`
   - Set Type: `iskoces`
   - Set Secure: `false`

2. **Via Glooscap Operator Config:**
   ```yaml
   env:
   - name: TRANSLATION_SERVICE_ADDR
     value: "iskoces-service.iskoces.svc:50051"
   - name: TRANSLATION_SERVICE_TYPE
     value: "iskoces"
   - name: TRANSLATION_SERVICE_SECURE
     value: "false"
   ```

## gRPC Access

The route exposes the gRPC service for external access:

- **External gRPC access via Route**: 
  - Connect to: `https://iskoces.apps.<your-cluster-domain>` (port 443, default HTTPS)
  - Or: `http://iskoces.apps.<your-cluster-domain>` (port 80, HTTP)
  - The route terminates TLS at the edge and forwards gRPC traffic (over HTTP/2) to the backend service on port 50051
  - **Important**: Use the route hostname with port 443 (HTTPS) or 80 (HTTP). Do not specify port 50051 in the URL - the route handles forwarding to the backend port.
- **Internal access (within cluster)**: Use the service directly: `iskoces-service.iskoces.svc:50051`
- **HTTP health checks**: A separate route (`iskoces-http`) exposes port 5000 for health checks

**Example gRPC client connection**:
```bash
# Connect to route (external access)
grpcurl -plaintext iskoces.apps.ocp-ai-sno-2.rh.dasmlab.org:80 list

# Or with TLS (recommended)
grpcurl iskoces.apps.ocp-ai-sno-2.rh.dasmlab.org:443 list
```

**Note**: The route is configured with `insecureEdgeTerminationPolicy: Allow` to support gRPC over HTTP/2 without redirects that would break gRPC connections.

## Troubleshooting

### Pods not starting
- Check image pull secret: `oc get secret dasmlab-ghcr-pull -n iskoces`
- Check PVC status: `oc get pvc -n iskoces`
- Check pod events: `oc describe pod <pod-name> -n iskoces`

### Models not loading
- Check logs: `oc logs -f deployment/iskoces-server -n iskoces`
- Verify PVC has enough space: `oc get pvc iskoces-models -n iskoces`
- Check ConfigMap: `oc get configmap iskoces-config -n iskoces -o yaml`

### Service not accessible
- Verify service selector matches pod labels: `oc get svc iskoces-service -n iskoces -o yaml`
- Check endpoints: `oc get endpoints iskoces-service -n iskoces`

## Cleanup

To remove Iskoces:

```bash
oc delete -f .
```

Or delete the namespace (removes everything):

```bash
oc delete namespace iskoces
```

