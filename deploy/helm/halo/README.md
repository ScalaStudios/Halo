# Halo Helm chart

Deploys the Halo server and the web interface, with an optional PostgreSQL 17. The chart reads every secret from Kubernetes Secrets you create, never from values.

## Install

Create a Secret for Halo. Its keys become the server's environment variables, so it must hold `HALO_SECRET_KEY` and `HALO_DATABASE_URL`, and can hold others such as `HALO_SMTP_PASSWORD`, `HALO_S3_SECRET_ACCESS_KEY` or `HALO_LOG_EXPORT_HEADERS`.

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

A running server reads the Secret only when it starts, so after adding or changing `HALO_METRICS_TOKEN` restart it with `kubectl -n halo rollout restart deployment/halo-server`, then check that a scrape without the token gets 401.

## Keys in HashiCorp Vault

To keep `HALO_SECRET_KEY` wrapped by, and the signing keys inside, HashiCorp Vault Transit, put the Vault-encrypted key in the Secret as `HALO_SECRET_KEY`, add `HALO_VAULT_TOKEN` to it, and set the `vault` values:

```sh
kubectl -n halo create secret generic halo \
  --from-literal=HALO_SECRET_KEY="$(vault write -field=ciphertext transit/encrypt/halo-secret-key plaintext="$(openssl rand -base64 32)")" \
  --from-literal=HALO_VAULT_TOKEN='TOKEN' \
  --from-literal=HALO_DATABASE_URL='postgres://halo:PASSWORD@db.example.internal:5432/halo'

helm install halo deploy/helm/halo -n halo \
  --set publicUrl=https://auth.example.com \
  --set existingSecret=halo \
  --set vault.address=https://vault.example.com:8200 \
  --set vault.wrappingKey=halo-secret-key \
  --set vault.signing=true
```

The server doesn't start while Vault is unreachable. The [main README](../../../README.md#keys-in-hashicorp-vault) lists the Vault policy, how to move existing keys and how to keep the token renewed with Vault Proxy.

## LDAP

Set `ldap.enabled=true` to run the read-only LDAP server on port 6360 of the container and the `halo-server` Service. It serves LDAPS with the certificate in a `kubernetes.io/tls` Secret, such as one cert-manager issues for the `publicUrl` host:

```sh
helm upgrade halo deploy/helm/halo -n halo --reuse-values \
  --set ldap.enabled=true \
  --set ldap.certificateSecret=halo-ldap-tls
```

The server runs without root and with every capability dropped, so keep the port at 1024 or higher. The ingress never routes to it: expose the port to the applications that need it with a LoadBalancer Service or your ingress controller's TCP routing. The server reads the certificate only when it starts, so restart it after the Secret is renewed.

## Upgrades

Change the image tags, or upgrade to a newer chart, then run `helm upgrade`. The server applies database migrations when it starts and reports ready on `/readyz` only afterwards. With one replica, the old pod stops before the new one starts. With `server.replicaCount` above 1, the chart replaces pods one at a time, starting each new pod before it stops an old one, and adds a PodDisruptionBudget that keeps all but one running during node drains. Back up the database and `HALO_SECRET_KEY` first.

## Values

| Value | Default | Description |
| --- | --- | --- |
| `publicUrl` | | Required. `HALO_PUBLIC_URL`, the address people use to reach Halo and the ingress host. |
| `existingSecret` | | Required. Secret whose keys are loaded as server environment variables. |
| `organization` | | `HALO_ORGANIZATION`. |
| `trustedProxies` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | `HALO_TRUSTED_PROXIES`. Narrow it to your pod CIDR. |
| `imagePullSecrets` | `[]` | Pull secrets for every pod. |
| `server.replicaCount` | `1` | Server replicas. Above 1, upgrades roll one pod at a time and a PodDisruptionBudget allows one pod down; see [Running several instances](../../../README.md#running-several-instances). |
| `server.image.*` | `ghcr.io/scalastudios/halo-server`, chart `appVersion` | Server image. |
| `server.env` | `{}` | Other non-secret server variables, such as `HALO_SMTP_HOST`, `HALO_SMTP_FROM` or `HALO_S3_ENDPOINT`. |
| `server.resources`, `server.podSecurityContext`, `server.securityContext`, `server.nodeSelector`, `server.tolerations`, `server.affinity` | | Server pod settings. |
| `web.replicaCount` | `1` | Web interface replicas. |
| `web.image.*` | `ghcr.io/scalastudios/halo-web`, chart `appVersion` | Web image. |
| `web.resources`, `web.podSecurityContext`, `web.securityContext`, `web.nodeSelector`, `web.tolerations`, `web.affinity` | | Web pod settings. |
| `radius.enabled` | `false` | Run Halo's RADIUS server, `HALO_RADIUS_LISTEN`, on its own UDP container port and a separate `<release>-radius` Service. |
| `radius.port` | `1812` | RADIUS port. The Service also forwards the next port, 1813 by default, to it for accounting. |
| `radius.serviceType` | `LoadBalancer` | Type of the RADIUS Service. `LoadBalancer` and `NodePort` get `externalTrafficPolicy: Local`, because Halo recognizes RADIUS clients by their source address. |
| `vault.address` | | `HALO_VAULT_ADDR`. Put `HALO_VAULT_TOKEN` in `existingSecret`. |
| `vault.namespace` | | `HALO_VAULT_NAMESPACE`, on Vault Enterprise and HCP Vault Dedicated. |
| `vault.transitMount` | | `HALO_VAULT_TRANSIT_MOUNT`, `transit` when empty. |
| `vault.wrappingKey` | | `HALO_VAULT_WRAPPING_KEY`. `HALO_SECRET_KEY` in `existingSecret` then holds the key encrypted by Vault. |
| `vault.signing` | `false` | `HALO_VAULT_SIGNING`: create new signing keys in Vault. |
| `metrics.enabled` | `false` | Serve Prometheus metrics on their own container and Service port, `HALO_METRICS_LISTEN`. The ingress never routes to it. |
| `metrics.port` | `9090` | Metrics port. |
| `metrics.serviceMonitor.enabled` | `false` | Create a Prometheus Operator ServiceMonitor. |
| `metrics.serviceMonitor.interval` | `30s` | Scrape interval. |
| `metrics.serviceMonitor.tokenKey` | | Key in `existingSecret` holding `HALO_METRICS_TOKEN`, usually `HALO_METRICS_TOKEN`. The ServiceMonitor sends it as a bearer token. |
| `ldap.enabled` | `false` | Serve the read-only LDAP directory on its own container and Service port, `HALO_LDAP_LISTEN`. |
| `ldap.port` | `6360` | LDAP port, 1024 or higher. |
| `ldap.tls` | `ldaps` | `HALO_LDAP_TLS`: `ldaps`, `starttls`, or `none` to accept passwords without TLS. |
| `ldap.certificateSecret` | | `kubernetes.io/tls` Secret with the certificate and key, mounted for `HALO_LDAP_CERT_FILE` and `HALO_LDAP_KEY_FILE`. Required unless `ldap.tls` is `none`. |
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
