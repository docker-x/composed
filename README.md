<p align="center">
  <img src="logo.svg" width="128" height="128" alt="composed logo">
</p>

<h1 align="center">composed</h1>

<p align="center">
  <strong>Compose anything into a Docker Compose file.</strong><br>
  Helm charts + Docker images + compose files → one <code>docker-compose.yaml</code>
</p>

<p align="center">
  <a href="https://github.com/docker-x/composed/releases"><img alt="Release" src="https://img.shields.io/github/v/release/docker-x/composed?style=flat-square"></a>
  <a href="https://github.com/docker-x/composed/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/docker-x/composed?style=flat-square"></a>
  <a href="https://goreportcard.com/report/github.com/docker-x/composed"><img alt="Go Report" src="https://goreportcard.com/badge/github.com/docker-x/composed?style=flat-square"></a>
</p>

---

**composed** lets you define a stack of Helm charts, plain Docker images, and existing compose files in a single config — then builds a merged `docker-compose.yaml` that runs on plain Docker. No Kubernetes cluster needed.

The config file is a **standard Docker Compose file** extended with [`x-` extensions](https://docs.docker.com/reference/compose-file/extension/). Plain services work with `docker compose up` directly. Services with `x-helm` need `composed build` to resolve.

## Quick Start

```bash
# Install
go install github.com/docker-x/composed@latest

# Create a stack
composed init --project my-stack
composed add oci://docker.litellm.ai/berriai/litellm-helm --set image.tag=main-stable
composed add postgres:15-alpine --port 5432:5432 --env POSTGRES_PASSWORD=secret

# Build and start
composed up
```

## How It Works

```yaml
# composed.yaml — it's just a compose file with x- extensions
name: my-stack

services:
  # Plain compose service — zero new syntax
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_PASSWORD: secret
    ports:
      - "5432:5432"
    x-exports:
      host: postgres
      password: secret

  # Helm chart — rendered and translated to compose services
  litellm:
    x-helm:
      chart: oci://docker.litellm.ai/berriai/litellm-helm
      values:
        image:
          tag: main-stable
    depends_on:
      - postgres

  # Include an external compose file
  monitoring:
    x-compose-file: ./monitoring/docker-compose.yaml
```

`composed build` resolves `x-helm` (renders chart, translates K8s → Compose), merges `x-compose-file` includes, and resolves `x-exports` cross-references → outputs a clean `docker-compose.yaml`.

## Extensions

`composed.yaml` adds three `x-` extensions to Docker Compose:

| Extension | Purpose |
|-----------|---------|
| **`x-helm`** | Render a Helm chart into compose services |
| **`x-compose-file`** | Include and merge an external docker-compose.yaml |
| **`x-exports`** | Expose values to other services via `${service.key}` |

Everything else is standard compose syntax. Docker Compose [ignores `x-` fields](https://docs.docker.com/reference/compose-file/extension/), so plain image services work with `docker compose up` directly.

### `x-helm`

```yaml
services:
  redis:
    x-helm:
      chart: bitnami/redis                          # OCI ref, repo/name, or local path
      repo: https://charts.bitnami.com/bitnami      # chart repository (optional)
      version: "18.x"                                # version constraint (optional)
      values:                                        # inline values (→ --set)
        architecture: standalone
      values_file: ./redis-values.yaml               # values file (→ helm -f)
```

### `x-compose-file`

```yaml
services:
  monitoring:
    x-compose-file: ./monitoring/docker-compose.yaml
```

### `x-exports`

```yaml
services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_PASSWORD: secret
    x-exports:
      host: postgres
      password: secret

  app:
    image: my-app:latest
    environment:
      DATABASE_URL: "postgresql://postgres:${postgres.password}@${postgres.host}/mydb"
```

## CLI Reference

### `composed init`

```bash
composed init                        # project name = directory name
composed init --project my-stack     # explicit project name
composed init --helm-values          # scaffold values files for x-helm services
```

### `composed add`

Type is auto-detected from the source:

| Source | Detection |
|--------|-----------|
| `oci://...` | Probes OCI manifest — helm chart or container image |
| `*.yaml` / `*.yml` | Compose file include |
| Directory with `Chart.yaml` | Local helm chart |
| Everything else | Docker image |

```bash
composed add oci://docker.litellm.ai/berriai/litellm-helm
composed add postgres:15-alpine --port 5432:5432 --env POSTGRES_PASSWORD=secret
composed add myapp:latest --depends-on postgres --depends-on redis
```

#### Helm values (3 ways)

```bash
# Inline (stored in composed.yaml)
composed add oci://... --set image.tag=main-stable

# Merge file contents inline at add time
composed add oci://... --values values.yaml

# Store reference, loaded at build time
composed add oci://... --values-file ./values.yaml
```

### `composed build`

```bash
composed build                    # finds composed.yaml walking up from cwd
composed build -o -               # stdout
```

### `composed up` / `composed down`

```bash
composed up      # build + docker compose up -d
composed down    # docker compose down
```

## What Translates (Helm Charts)

| K8s Resource | Compose Mapping |
|---|---|
| Deployment / StatefulSet / DaemonSet | `services:` entry |
| Service (NodePort/LoadBalancer) | `ports:` on matched service |
| ConfigMap | `environment:` or `configs:` |
| Secret | `environment:` (base64-decoded) |
| PersistentVolumeClaim | Named `volumes:` |
| Job | One-shot service (`restart: on-failure`) |
| Init containers | Separate service + `depends_on` chain |
| Probes | `healthcheck:` |
| Resource limits | `deploy.resources:` |

ServiceAccount, RBAC, NetworkPolicy, Ingress, HPA, CRDs are skipped (no Compose equivalent).

## Container Labels

All output services are labeled for easy identification:

```bash
docker ps --filter label=com.composed.managed
docker ps --filter label=com.composed.project=my-stack
```

## Examples

| Example | Description |
|---------|-------------|
| [`litellm-chart/`](examples/litellm-chart/) | Minimal — OCI chart in 3 commands |
| [`litellm-chart-with-values/`](examples/litellm-chart-with-values/) | OCI chart + scaffolded values file |
| [`litellm-chart-local/`](examples/litellm-chart-local/) | Local wrapper chart pattern |
| [`litellm-n8n-shared-pg/`](examples/litellm-n8n-shared-pg/) | Shared Postgres with schema isolation + cross-refs |

## Part of Docker eXtra

**composed** is a [Docker eXtra](https://github.com/docker-x) project — tools that extend Docker Compose with superpowers.

## License

Apache 2.0 — see [LICENSE](LICENSE).
