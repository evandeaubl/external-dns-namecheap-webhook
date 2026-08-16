# external-dns-namecheap-webhook

A [webhook provider](https://kubernetes-sigs.github.io/external-dns/v0.21.0/docs/tutorials/webhook-provider/) for
[external-dns](https://github.com/kubernetes-sigs/external-dns) that integrates with the
[Namecheap](https://www.namecheap.com/support/api/intro/) DNS API.

The webhook runs as a **sidecar container** alongside the external-dns container in the same Pod.
external-dns communicates with it over HTTP on `localhost:8888`.

## Features

- Implements the external-dns webhook provider interface (`/` negotiate, `/records` get/apply,
  `/adjustendpoints`)
- Exposes `/healthz` on port 8080 for liveness and readiness probes
- Defaults to the **Namecheap sandbox** environment; production requires an explicit flag
- All configuration available via command-line flags or environment variables
- Sensitive values (API key, API user, etc.) only accepted via environment variables

## Building

### Container image

A multi-stage `Dockerfile` is included. It builds the Go binary and packages it into a
[distroless](https://github.com/GoogleContainerTools/distroless) image.

```sh
docker build -t namecheap-webhook:latest .
```

Or with podman:

```sh
podman build -t namecheap-webhook:latest .
```

### Local build

```sh
go build -o namecheap-webhook ./cmd/namecheap-webhook
```

## Configuration

### Namecheap credentials

You need four values from your [Namecheap API access](https://www.namecheap.com/my/account/api-access/)
page:

| Environment variable       | Description                                                                 |
|----------------------------|-----------------------------------------------------------------------------|
| `NAMECHEAP_API_USER`       | Your Namecheap API user name (usually your Namecheap account name).        |
| `NAMECHEAP_API_KEY`        | Your Namecheap API key.                                                    |
| `NAMECHEAP_USERNAME`       | Your Namecheap username (defaults to `NAMECHEAP_API_USER`).                |
| `NAMECHEAP_CLIENT_IP`      | Client IP for the Namecheap API (defaults to `127.0.0.1`). |

### Environment variables and flags

All options can be set as command-line flags or environment variables. Sensitive values
marked with **(env only)** can only be passed via environment variables.

| Flag                    | Environment variable      | Default                              | Description                                                                 |
|-------------------------|---------------------------|--------------------------------------|-----------------------------------------------------------------------------|
| `--api-user`            | `NAMECHEAP_API_USER`      | *(required)*                         | Namecheap API user **(env only)**.                                          |
| `--api-key`             | `NAMECHEAP_API_KEY`       | *(required)*                         | Namecheap API key **(env only)**.                                           |
| `--username`            | `NAMECHEAP_USERNAME`      | *(api-user)*                         | Namecheap username, defaults to api-user **(env only)**.                    |
| `--client-ip`           | `NAMECHEAP_CLIENT_IP`     | `127.0.0.1`                          | Client IP for Namecheap API.                                                |
| `--production`          | `NAMECHEAP_PRODUCTION`    | `false`                              | Use the Namecheap production API instead of the sandbox.                    |
| `--domain-filter`       | `NAMECHEAP_DOMAIN_FILTER` | *(none)*                             | Comma-separated list of domains to manage.                                  |
| `--listen-address`      | `LISTEN_ADDRESS`          | `:8888`                              | Address for the webhook server (listen on `localhost` in production).        |
| `--healthz-address`     | `HEALTHZ_ADDRESS`         | `:8080`                              | Address for the health/metrics server.                                      |
| `--request-ttl`         | `REQUEST_TTL`             | `60s`                                | Timeout for Namecheap API requests.                                         |

### API endpoints

| Endpoint             | Method | Port | Path               | Purpose                         |
|----------------------|--------|------|--------------------|---------------------------------|
| Webhook (external-dns) | GET/POST | 8888 | `/`, `/records`, `/adjustendpoints` | external-dns webhook interface |
| Health check          | GET    | 8080 | `/healthz`         | Liveness and readiness probe    |

## Running

```sh
docker run --rm \
  -e NAMECHEAP_API_USER=... \
  -e NAMECHEAP_API_KEY=... \
  -e NAMECHEAP_USERNAME=... \
  -e NAMECHEAP_CLIENT_IP=... \
  -p 8888:8888 \
  -p 8080:8080 \
  namecheap-webhook:latest
```

`NAMECHEAP_USERNAME` defaults to `NAMECHEAP_API_USER` and `NAMECHEAP_CLIENT_IP` defaults to `127.0.0.1`.

## Kubernetes deployment

### Prerequisites

1. Create a Kubernetes secret with your Namecheap credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: namecheap-credentials
type: Opaque
stringData:
  NAMECHEAP_API_USER: "your-api-user"
  NAMECHEAP_API_KEY: "your-api-key"
  NAMECHEAP_USERNAME: "your-username"   # optional, defaults to NAMECHEAP_API_USER
  NAMECHEAP_CLIENT_IP: "127.0.0.1"
```

2. Build and push the container image to your registry (or use the pre-built
   `evandeaubl/external-dns-namecheap-webhook:latest` image directly):

```sh
# Option A: use the pre-built image (no action needed)

# Option B: build and push your own
docker build -t <your-registry>/external-dns-namecheap-webhook:latest .
docker push <your-registry>/external-dns-namecheap-webhook:latest
```

If you build your own image, substitute `<your-registry>/external-dns-namecheap-webhook:latest`
for `evandeaubl/external-dns-namecheap-webhook:latest` in the examples below.

### Deploying with the external-dns Helm chart

The [external-dns Helm chart](https://github.com/kubernetes-sigs/external-dns/tree/master/charts/external-dns)
natively supports a webhook provider sidecar via the `provider.webhook.*` values.

Install the chart with the Namecheap webhook configured as a sidecar:

```sh
helm repo add external-dns https://kubernetes-sigs.github.io/external-dns/
helm repo update

helm upgrade --install external-dns external-dns/external-dns \
  --set provider.name=webhook \
  --set provider.webhook.image.repository=evandeaubl/external-dns-namecheap-webhook \
  --set provider.webhook.image.tag=latest \
  --set provider.webhook.image.pullPolicy=IfNotPresent \
  --set provider.webhook.env[0].name=NAMECHEAP_API_USER \
  --set provider.webhook.env[0].valueFrom.secretKeyRef.name=namecheap-credentials \
  --set provider.webhook.env[0].valueFrom.secretKeyRef.key=NAMECHEAP_API_USER \
  --set provider.webhook.env[1].name=NAMECHEAP_API_KEY \
  --set provider.webhook.env[1].valueFrom.secretKeyRef.name=namecheap-credentials \
  --set provider.webhook.env[1].valueFrom.secretKeyRef.key=NAMECHEAP_API_KEY \
  --set provider.webhook.env[2].name=NAMECHEAP_USERNAME \
  --set provider.webhook.env[2].valueFrom.secretKeyRef.name=namecheap-credentials \
  --set provider.webhook.env[2].valueFrom.secretKeyRef.key=NAMECHEAP_USERNAME \
  --set provider.webhook.env[3].name=NAMECHEAP_CLIENT_IP \
  --set provider.webhook.env[3].valueFrom.secretKeyRef.name=namecheap-credentials \
  --set provider.webhook.env[3].valueFrom.secretKeyRef.key=NAMECHEAP_CLIENT_IP \
  --set provider.webhook.args="--listen-address=127.0.0.1:8888" \
  --set extraArgs.provider=webhook
```

For a more maintainable setup, use a `values.yaml` file:

```yaml
# values.yaml
provider:
  name: webhook
  webhook:
    image:
      repository: evandeaubl/external-dns-namecheap-webhook
      tag: latest
      pullPolicy: IfNotPresent
    args:
      - --listen-address=127.0.0.1:8888
    env:
      - name: NAMECHEAP_API_USER
        valueFrom:
          secretKeyRef:
            name: namecheap-credentials
            key: NAMECHEAP_API_USER
      - name: NAMECHEAP_API_KEY
        valueFrom:
          secretKeyRef:
            name: namecheap-credentials
            key: NAMECHEAP_API_KEY
      - name: NAMECHEAP_USERNAME
        valueFrom:
          secretKeyRef:
            name: namecheap-credentials
            key: NAMECHEAP_USERNAME
      - name: NAMECHEAP_CLIENT_IP
        valueFrom:
          secretKeyRef:
            name: namecheap-credentials
            key: NAMECHEAP_CLIENT_IP
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8080
      initialDelaySeconds: 30
      periodSeconds: 30
    readinessProbe:
      httpGet:
        path: /healthz
        port: 8080
      initialDelaySeconds: 15
      periodSeconds: 15

extraArgs:
  provider: webhook

# Optional: restrict which domains external-dns will manage
domainFilters:
  - example.com
```

Then install:

```sh
helm upgrade --install external-dns external-dns/external-dns -f values.yaml
```

### Manual Kubernetes manifest

If you prefer not to use the Helm chart, here is a standalone manifest:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns
spec:
  replicas: 1
  selector:
    matchLabels:
      app: external-dns
  template:
    metadata:
      labels:
        app: external-dns
    spec:
      serviceAccountName: external-dns
      containers:
        # external-dns container
        - name: external-dns
          image: registry.k8s.io/external-dns/external-dns:v0.21.0
          args:
            - --provider=webhook
            - --source=service
            - --source=ingress
            - --interval=1m
            - --policy=upsert-only
          ports:
            - containerPort: 7979
              name: metrics

        # Namecheap webhook sidecar
        - name: namecheap-webhook
          image: evandeaubl/external-dns-namecheap-webhook:latest
          args:
            - --listen-address=127.0.0.1:8888
          env:
            - name: NAMECHEAP_API_USER
              valueFrom:
                secretKeyRef:
                  name: namecheap-credentials
                  key: NAMECHEAP_API_USER
            - name: NAMECHEAP_API_KEY
              valueFrom:
                secretKeyRef:
                  name: namecheap-credentials
                  key: NAMECHEAP_API_KEY
            - name: NAMECHEAP_USERNAME
              valueFrom:
                secretKeyRef:
                  name: namecheap-credentials
                  key: NAMECHEAP_USERNAME
            - name: NAMECHEAP_CLIENT_IP
              valueFrom:
                secretKeyRef:
                  name: namecheap-credentials
                  key: NAMECHEAP_CLIENT_IP
          ports:
            - containerPort: 8888
              name: webhook
            - containerPort: 8080
              name: healthz
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 30
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 15
            periodSeconds: 15
```

## License

See the external-dns project for licensing of the upstream components. This webhook
implementation is provided as-is.
