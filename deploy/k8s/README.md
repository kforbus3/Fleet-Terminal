# Fleet Terminal — Kubernetes manifests

Raw, ordered manifests for a from-scratch deploy. For a parameterized install
use the Helm chart in `../helm/fleet-terminal` instead.

## Apply order

Files are number-prefixed so `kubectl apply -f .` resolves dependencies:

```sh
kubectl apply -f deploy/k8s/
```

| File                 | Resource                                          |
|----------------------|---------------------------------------------------|
| `00-namespace.yaml`  | Namespace (Pod Security: restricted)              |
| `10-configmap.yaml`  | Non-secret backend config                         |
| `11-secret.yaml`     | **Templated** secrets — replace before applying   |
| `20-postgres.yaml`   | Postgres StatefulSet + headless Service + PVC     |
| `21-redis.yaml`      | Redis Deployment + Service                        |
| `30-backend.yaml`    | Backend Deployment + Service + HPA                |
| `31-frontend.yaml`   | Frontend Deployment + Service                     |
| `40-ingress.yaml`    | TLS Ingress (host + cert-manager)                 |

## Before you apply

1. Edit `10-configmap.yaml` → set `FLEET_PUBLIC_URL` to your real host.
2. Replace `11-secret.yaml` placeholders, or create the secret out-of-band:

   ```sh
   kubectl -n fleet-terminal create secret generic fleet-secrets \
     --from-literal=FLEET_JWT_SECRET="$(openssl rand -hex 32)" \
     --from-literal=FLEET_CSRF_SECRET="$(openssl rand -hex 32)" \
     --from-literal=FLEET_CA_PASSPHRASE="$(openssl rand -hex 32)" \
     --from-literal=POSTGRES_PASSWORD="$(openssl rand -hex 24)" \
     --from-literal=FLEET_DATABASE_URL="postgres://fleet:THE_SAME_PASSWORD@fleet-postgres:5432/fleet?sslmode=disable"
   ```

3. Update the host in `40-ingress.yaml` (and the TLS `secretName`).
4. Push images to `ghcr.io/fleet-terminal/{backend,frontend}` or edit the
   `image:` fields to point at your registry.

## Probes & scaling

- Backend readiness: `GET /ready`, liveness: `GET /health`, metrics: `GET /metrics`.
- Prometheus scrape annotations are set on the backend Service and Pods.
- The backend HPA scales 2→10 on CPU (70%) / memory (80%).

## Security posture

All workloads run `runAsNonRoot`, drop all capabilities, disable privilege
escalation, and use `readOnlyRootFilesystem: true` where the image permits
(backend, redis, frontend). Postgres keeps a writable data volume.
