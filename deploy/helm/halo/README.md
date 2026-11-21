# Halo Helm chart

Deploys the Halo server and the web interface, with an optional PostgreSQL 17. The chart reads every secret from Kubernetes Secrets you create, never from values.

## Install

Create a Secret for Halo. Its keys become the server's environment variables, so it must hold `HALO_SECRET_KEY` and `HALO_DATABASE_URL`, and can hold others such as `HALO_SMTP_PASSWORD` or `HALO_S3_SECRET_ACCESS_KEY`.

```sh
kubectl create namespace halo
kubectl -n halo create secret generic halo \
  --from-literal=HALO_SECRET_KEY="$(openssl rand -base64 32)" \
  --from-literal=HALO_DATABASE_URL='postgres://halo:PASSWORD@db.example.internal:5432/halo'
```

Then install from a checkout:

```sh
helm install halo deploy/helm/halo -n halo \
  --set publicUrl=https://auth.example.com \
  --set existingSecret=halo \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.tlsSecretName=halo-tls \
  --set-string 'ingress.annotations.cert-manager\.io/cluster-issuer=letsencrypt'
```

Create the first administrator:

```sh
kubectl -n halo exec deploy/halo-server -- halo bootstrap --email you@example.com --name "Your Name"
```

## Bundled PostgreSQL

Set `postgresql.enabled=true` to run PostgreSQL 17 as a StatefulSet in the release. Its password comes from `postgresql.existingSecret`, and `HALO_DATABASE_URL` in the Halo Secret points at the `<release>-postgresql` Service:

```sh
password="$(openssl rand -hex 32)"
kubectl -n halo create secret generic halo-postgresql --from-literal=password="$password"
kubectl -n halo create secret generic halo \
  --from-literal=HALO_SECRET_KEY="$(openssl rand -base64 32)" \
  --from-literal=HALO_DATABASE_URL="postgres://halo:$password@halo-postgresql:5432/halo"
helm install halo deploy/helm/halo -n halo \
  --set publicUrl=https://auth.example.com \
  --set existingSecret=halo \
  --set postgresql.enabled=true \
  --set postgresql.existingSecret=halo-postgresql
```

A managed PostgreSQL is the better choice for production.

## Metrics

Set `metrics.enabled=true` to serve Prometheus metrics on port 9090 of the `halo-server` Service. To require a token, add `HALO_METRICS_TOKEN` to the Halo Secret, which loads it into the server, and point the ServiceMonitor at the same key:

```sh
helm upgrade halo deploy/helm/halo -n halo --reuse-values \
  --set metrics.enabled=true \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.serviceMonitor.tokenKey=HALO_METRICS_TOKEN
```

## Upgrades

Change the image tags, or upgrade to a newer chart, then run `helm upgrade`. The server applies database migrations when it starts and reports ready only afterwards. It runs as a single replica that is stopped before the new one starts, because background jobs run in every server process. Back up the database and `HALO_SECRET_KEY` first.

## Values

| Value | Default | Description |
| --- | --- | --- |
| `publicUrl` | | Required. `HALO_PUBLIC_URL`, the address people use to reach Halo and the ingress host. |
| `existingSecret` | | Required. Secret whose keys are loaded as server environment variables. |
| `organization` | | `HALO_ORGANIZATION`. |
| `trustedProxies` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | `HALO_TRUSTED_PROXIES`. Narrow it to your pod CIDR. |
| `imagePullSecrets` | `[]` | Pull secrets for every pod. |
| `server.image.*` | `ghcr.io/scalastudios/halo-server`, chart `appVersion` | Server image. |
| `server.env` | `{}` | Other non-secret server variables, such as `HALO_SMTP_HOST`, `HALO_SMTP_FROM` or `HALO_S3_ENDPOINT`. |
| `server.resources`, `server.podSecurityContext`, `server.securityContext`, `server.nodeSelector`, `server.tolerations`, `server.affinity` | | Server pod settings. |
| `web.replicaCount` | `1` | Web interface replicas. |
| `web.image.*` | `ghcr.io/scalastudios/halo-web`, chart `appVersion` | Web image. |
| `web.resources`, `web.podSecurityContext`, `web.securityContext`, `web.nodeSelector`, `web.tolerations`, `web.affinity` | | Web pod settings. |
| `metrics.enabled` | `false` | Serve Prometheus metrics on their own container and Service port, `HALO_METRICS_LISTEN`. The ingress never routes to it. |
| `metrics.port` | `9090` | Metrics port. |
| `metrics.serviceMonitor.enabled` | `false` | Create a Prometheus Operator ServiceMonitor. |
| `metrics.serviceMonitor.interval` | `30s` | Scrape interval. |
| `metrics.serviceMonitor.tokenKey` | | Key in `existingSecret` holding `HALO_METRICS_TOKEN`, usually `HALO_METRICS_TOKEN`. The ServiceMonitor sends it as a bearer token. |
| `ingress.enabled` | `false` | Create an Ingress for the `publicUrl` host that routes to the web interface. |
| `ingress.className` | | Ingress class. |
| `ingress.annotations` | `{}` | For example a cert-manager issuer. |
| `ingress.tlsSecretName` | | TLS Secret for the host; TLS is off when empty. |
| `postgresql.enabled` | `false` | Run PostgreSQL 17 in the release. |
| `postgresql.existingSecret`, `postgresql.passwordKey` | `password` | Secret and key holding the database password. |
| `postgresql.image.*` | `postgres:17-alpine` | PostgreSQL image. |
| `postgresql.persistence.size`, `postgresql.persistence.storageClass` | `8Gi` | Data volume. |
| `postgresql.resources`, `postgresql.podSecurityContext`, `postgresql.securityContext` | | PostgreSQL pod settings. |

## Limitations

- The published web image proxies API requests to `http://halo-server:8080`, so the server Service is always named `halo-server` and a namespace holds one Halo release.
- The server always runs one replica.
