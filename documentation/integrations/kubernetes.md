# Kubernetes

Sign in to Kubernetes clusters with `kubectl` through Halo, and grant cluster permissions to Halo groups with RBAC. `kubectl` gets its token from [kubelogin](https://github.com/int128/kubelogin), the `kubectl oidc-login` plugin, which opens Halo in the browser when a token is needed. The cluster's API server verifies Halo's ID tokens.

The examples use `https://auth.example.com` for Halo.

## Register the cluster in Halo

kubectl runs on each engineer's machine, so register it as a native application. It gets no client secret and signs in with PKCE.

1. In the console, open **Applications** and choose **Add application**.
2. Choose **OpenID Connect** and **Continue**.
3. Choose **Native or CLI** and **Continue**.
4. Name the application after the cluster, for example `Kubernetes · prod`, and enter this redirect URI:

   ```text
   http://localhost:8000
   ```

5. Choose **Create application** and copy the client ID.
6. Choose **Open application**, go to **Users & groups**, choose **Assign group**, and pick the groups whose members may use the cluster.

kubelogin listens on port 8000 and falls back to port 18000 when 8000 is busy. Halo accepts any port on a loopback redirect URI for native applications, so the one URI covers both.

The console shows the generic endpoints for native applications. The kubectl configuration below is the one the console shows for applications registered with the `kubernetes` setup guide through the [API](../api.md).

## Configure the API server

Add these flags to `kube-apiserver`, with your client ID:

```text
--oidc-issuer-url=https://auth.example.com
--oidc-client-id=hl_…
--oidc-username-claim=email
--oidc-groups-claim=groups
--oidc-groups-prefix=halo:
```

- `--oidc-issuer-url` must be Halo's address exactly. The API server accepts only https issuers and fetches Halo's discovery document and signing keys from it, so it must be able to reach Halo.
- `--oidc-username-claim=email` makes the person's email address their Kubernetes username. Kubernetes rejects tokens whose `email_verified` claim is false, so the person's address must be verified in Halo; [Claims](../security-model.md#claims) lists how an address becomes verified.
- `--oidc-groups-prefix=halo:` puts `halo:` in front of every group name, so a Halo group can never be mistaken for a built-in group such as `system:masters`.

Halo signs ID tokens with RS256, the API server's default. How you set the flags depends on how the cluster was installed; on kubeadm clusters they go in `/etc/kubernetes/manifests/kube-apiserver.yaml`. The API server refuses to start if you combine these flags with a structured `--authentication-config` file; use one or the other.

## Grant permissions to Halo groups

Halo's `groups` claim lists the names of all of the person's groups, and with the prefix above, a Halo group named `Kubernetes admins` arrives as `halo:Kubernetes admins`. Bind it to a role:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: halo-kubernetes-admins
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: Group
  name: "halo:Kubernetes admins"
```

Use the group name exactly as it appears in Halo's console, with the prefix.

## Configure kubectl

Each engineer installs kubelogin and adds this user to their kubeconfig (`~/.kube/config`), with your client ID:

```yaml
users:
- name: halo
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: kubectl
      args:
      - oidc-login
      - get-token
      - --oidc-issuer-url=https://auth.example.com
      - --oidc-client-id=hl_…
      - --oidc-extra-scope=email
      - --oidc-extra-scope=groups
      interactiveMode: IfAvailable
```

Then point a context at the cluster with this user:

```bash
kubectl config set-context prod --cluster=prod --user=halo
kubectl config use-context prod
```

No client secret is needed: kubelogin uses PKCE, which Halo requires for native applications.

## Check it

Run any command, for example:

```bash
kubectl get nodes
```

kubelogin opens Halo in your browser. After you sign in, the command completes and kubelogin caches the token in `~/.kube/cache/oidc-login`. When the ID token expires after 15 minutes, kubelogin opens the browser again; while your Halo session lasts (12 hours), Halo returns a new token without asking you to sign in.

`kubectl auth whoami` shows the username and groups the API server derived from your token. If a command fails with `Forbidden`, check the group name in your binding against `halo:` plus the name in Halo. For sign-in errors, see [troubleshooting](generic-oidc.md#troubleshooting).
