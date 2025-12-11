# Iskoces Kubernetes Quick Start

This guide will help you deploy Iskoces to a Kubernetes cluster (k3d, minikube, or any other).

## Prerequisites

- Kubernetes cluster running (k3d, minikube, or other)
- `kubectl` configured to access the cluster
- Iskoces Docker image built and available (or use pre-built from registry)

## Quick Deployment

### Option 1: Standalone Deployment

Deploy Iskoces independently:

```bash
cd /path/to/iskoces/manifests
./deploy.sh
```

This will:
- Create `iskoces` namespace
- Deploy Iskoces server with persistent storage for models
- Expose gRPC service at `iskoces-service.iskoces.svc:50051`

### Option 2: Deploy with Glooscap

Deploy both Glooscap and Iskoces together:

```bash
cd /path/to/glooscap/infra/macos-foss
ISKOCES_DIR=/path/to/iskoces ./scripts/deploy-glooscap.sh
```

Then configure Glooscap to use Iskoces via the UI (Settings → Translation Service).

## Verify Deployment

Check that Iskoces is running:

```bash
kubectl get pods -n iskoces
kubectl get svc -n iskoces
```

View logs (models may take time to load on first startup):

```bash
kubectl logs -f -n iskoces deployment/iskoces-server
```

## Configure Glooscap to Use Iskoces

### Via UI (Recommended)

1. Port-forward Glooscap UI:
   ```bash
   kubectl port-forward -n glooscap-system svc/glooscap-ui 8080:80
   ```

2. Open http://localhost:8080 in your browser

3. Go to Settings → Translation Service tab

4. Configure:
   - **Address**: `iskoces-service.iskoces.svc:50051`
   - **Type**: `iskoces`
   - **Secure**: `false`

5. Click "Set Configuration"

### Via Environment Variables

Edit Glooscap operator deployment:

```bash
kubectl edit deployment glooscap-operator -n glooscap-system
```

Add environment variables:
```yaml
env:
- name: TRANSLATION_SERVICE_ADDR
  value: "iskoces-service.iskoces.svc:50051"
- name: TRANSLATION_SERVICE_TYPE
  value: "iskoces"
- name: TRANSLATION_SERVICE_SECURE
  value: "false"
```

## Customize Configuration

Edit `configmap.yaml` to change:
- Translation engine (`libretranslate` or `argos`)
- Languages to load (`en,fr,es` or `all`)
- Log level (`debug`, `info`, `warn`, `error`)

Then reapply:
```bash
kubectl apply -f configmap.yaml
kubectl rollout restart deployment/iskoces-server -n iskoces
```

## Undeploy

To remove Iskoces:

```bash
cd /path/to/iskoces/manifests
./undeploy.sh
```

To also delete models (PVC):
```bash
DELETE_PVC=true ./undeploy.sh
```

To delete everything including namespace:
```bash
DELETE_NAMESPACE=true DELETE_PVC=true ./undeploy.sh
```

## Troubleshooting

### Pod not starting

- Check logs: `kubectl logs -n iskoces deployment/iskoces-server`
- Check events: `kubectl describe pod -n iskoces -l app.kubernetes.io/name=iskoces`
- Verify image exists: `kubectl get pod -n iskoces -o jsonpath='{.items[0].spec.containers[0].image}'`

### Models not loading

- First startup can take 5-10 minutes to download models
- Check PVC is bound: `kubectl get pvc -n iskoces`
- Increase memory limits if needed (edit `deployment.yaml`)

### gRPC connection issues

- Verify service: `kubectl get svc -n iskoces`
- Test from within cluster: `kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- curl -v http://iskoces-service.iskoces.svc:5000/health`

