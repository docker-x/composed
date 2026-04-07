---
sidebar_position: 2
---

# Extensions

Composed extends the Docker Compose service format with `x-` prefixed fields. These extensions control how services are sourced and how they share configuration. Docker Compose ignores all `x-` fields, so the file remains valid compose syntax.

---

## x-helm

Declares that a service is backed by a Helm chart. At build time, Composed renders the chart with `helm template` and translates the resulting Kubernetes manifests into Docker Compose services.

```yaml
services:
  redis:
    x-helm:
      chart: bitnami/redis                       # OCI ref, repo/name, or local path
      repo: https://charts.bitnami.com/bitnami   # chart repository (optional)
      version: "18.x"                             # version constraint (optional)
      values:                                     # inline values (passed as --set)
        architecture: standalone
      values_file: ./redis-values.yaml            # values file (passed as -f to helm)
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `chart` | yes | Chart reference. Accepts OCI URIs (`oci://...`), repository-qualified names (`bitnami/redis`), or a local directory path containing `Chart.yaml`. |
| `repo` | no | Helm chart repository URL. Required when `chart` is a `repo/name` reference and the repository has not been added to Helm locally. |
| `version` | no | SemVer version constraint for the chart (e.g. `"18.x"`, `">=4.0.0"`). When omitted, the latest version is used. |
| `values` | no | Inline values map. Each key-value pair is passed to `helm template` as `--set key=value`. Nested maps are flattened with dots (`auth.enabled: false` becomes `--set auth.enabled=false`). |
| `values_file` | no | Path to a YAML values file, relative to `composed.yaml`. Passed to `helm template` as `-f`. Loaded at build time. |

When both `values` and `values_file` are set, the values file is loaded first and inline values override on top. See [Helm Values](helm-values.md) for the full priority rules.

---

## x-compose-file

Declares that a service is backed by an external Docker Compose file. At build time, Composed parses the referenced file and merges its services, volumes, networks, and configs into the output.

```yaml
services:
  monitoring:
    x-compose-file: ./monitoring/docker-compose.yaml
```

The path is resolved relative to the location of `composed.yaml`. The referenced file must be a valid Docker Compose file.

Everything defined in the external file is included in the final output:

- **Services** are merged by name. If the external file defines a service called `prometheus`, it appears as `prometheus` in the output.
- **Volumes** declared in the external file are added to the top-level `volumes:` section.
- **Networks** declared in the external file are added to the top-level `networks:` section.

This is useful for incorporating third-party or team-maintained compose stacks without copying their contents.

---

## x-exports

Defines key-value pairs that other services can reference. This enables loose coupling between services -- a database service exports its hostname and credentials, and the application service consumes them without hardcoding values.

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

### Reference syntax

Use `${service_name.key}` to reference an exported value. The placeholder is replaced with the literal value from the exporting service's `x-exports` map.

References are resolved by `composed build` before any rendering or merging takes place. They work in:

- **Service environment variables** -- the most common use case.
- **Helm inline values** (`x-helm.values`) -- pass exported values into chart configuration.

### Resolution rules

- All `x-exports` across all services are collected into a flat index of `service_name.key -> value`.
- Every `${service_name.key}` placeholder in environment values and helm inline values is replaced.
- If a placeholder references a service or key that does not exist, it is left as-is (no error is raised).
- Export values are always strings.

### Example: multi-service wiring

```yaml
services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: myapp
      POSTGRES_PASSWORD: dbpass123
    x-exports:
      host: postgres
      port: "5432"
      password: dbpass123

  redis:
    x-helm:
      chart: bitnami/redis
      repo: https://charts.bitnami.com/bitnami
      values:
        architecture: standalone
    x-exports:
      host: redis-master
      port: "6379"

  worker:
    image: my-worker:latest
    environment:
      DATABASE_URL: "postgresql://postgres:${postgres.password}@${postgres.host}:${postgres.port}/myapp"
      REDIS_URL: "redis://${redis.host}:${redis.port}"
    depends_on:
      - postgres
      - redis
```

---

## Standard compose fields

All standard Docker Compose service fields work as-is in `composed.yaml`. Composed passes them through to the output unchanged for image services and respects them during merging for all service types.

Commonly used fields:

| Field | Description |
|-------|-------------|
| `image` | Docker image reference (required for image services) |
| `environment` | Environment variables as a key-value map |
| `ports` | Port mappings (`"host:container"`) |
| `volumes` | Volume mounts (`"name:/path"` or `"/host:/container"`) |
| `command` | Override the default command |
| `entrypoint` | Override the default entrypoint |
| `healthcheck` | Container health check configuration |
| `labels` | Metadata labels on the container |
| `depends_on` | Service startup dependencies |
| `restart` | Restart policy (`no`, `on-failure`, `unless-stopped`, `always`) |

These fields are documented in the [Docker Compose specification](https://docs.docker.com/compose/compose-file/). Composed does not modify or validate them beyond what Docker Compose itself requires.
