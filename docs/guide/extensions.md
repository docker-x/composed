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

## x-k8s

Declares that a service is backed by Kubernetes YAML manifests. At build time, Composed reads the manifests from a directory or file and translates them into Docker Compose services using the same K8s-to-Compose pipeline that `x-helm` uses internally.

This supports any tool that produces standard K8s manifests: **cdk8s**, **kustomize**, **Timoni**, **Tanka**, or hand-written YAML.

```yaml
services:
  my-app:
    x-k8s:
      path: ./k8s/manifests          # directory of *.yaml files or a single file
```

With an optional pre-build command (e.g. cdk8s, kustomize):

```yaml
services:
  my-app:
    x-k8s:
      command: "cdk8s synth"          # run before reading manifests
      path: ./my-cdk8s-app/dist      # where to find K8s YAML after command runs
```

### Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `path` | yes | -- | Directory of K8s YAML files (globbed as `*.yaml` + `*.yml`) or a single YAML file. Resolved relative to `composed.yaml`. |
| `command` | no | -- | Shell command to run before reading manifests (e.g. `cdk8s synth`, `kustomize build -o dir/`). Runs with a 60-second timeout. |

### Execution rules

- If `command` is set, it runs before reading `path`.
- If `path` is a directory, all `*.yaml` and `*.yml` files in it are concatenated (non-recursive -- only top-level files).
- The concatenated YAML is fed through the same `k8s.Parse` -> `translate.Translate` pipeline used by `x-helm`.

### Relationship to x-helm

`x-helm` is effectively sugar for "run `helm template`, then do the standard K8s-to-Compose translation." `x-k8s` exposes the second half directly, accepting manifests from any source.

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

### Component pattern

Components should define infrastructure only -- image, healthcheck, ports, volumes. Credentials and configuration belong in the consumer's `composed.yaml`, passed via `environment:` or `env_file:`.

```yaml
# pgvector/compose.yaml -- reusable component (no credentials)
services:
  postgres:
    image: pgvector/pgvector:pg15
    ports:
      - "5432:5432"
    volumes:
      - pgvector-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready"]
```

```yaml
# composed.yaml -- consumer provides credentials
services:
  postgres:
    x-compose-file: ./pgvector/compose.yaml
    env_file:
      - ./postgres.env       # POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB

  app:
    image: my-app:latest
    environment:
      DATABASE_URL: "postgresql://${postgres.environment.POSTGRES_USER}:${postgres.environment.POSTGRES_PASSWORD}@${postgres.hostname}/mydb"
    depends_on:
      - postgres
```

At build time, Composed reads `env_file` entries and component `environment:` blocks to make values available for `${service.environment.KEY}` cross-references. Preloaded values are used only for resolution -- they are not duplicated in the output.

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

---

## Direct references

In addition to `x-exports`, you can reference fields from other services directly using path syntax. This eliminates the need to duplicate values in `x-exports` for plain image services.

### Supported paths

| Syntax | Resolves to | Example |
|--------|-------------|---------|
| `${svc.environment.KEY}` | Value of environment variable `KEY` in service `svc` | `${postgres.environment.POSTGRES_PASSWORD}` |
| `${svc.hostname}` | The service name (Compose DNS name) | `${postgres.hostname}` → `postgres` |
| `${svc.image}` | The `image` field of the service | `${postgres.image}` → `postgres:15` |
| `${svc.ports[N]}` | The Nth port mapping (0-indexed) | `${postgres.ports[0]}` → `5432:5432` |

### Priority

When both `x-exports` and a direct field match the same reference, **`x-exports` wins**. This lets you override or alias values:

```yaml
services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_PASSWORD: real-secret
    x-exports:
      password: exported-secret   # overrides direct lookup

  app:
    environment:
      # Resolves to "exported-secret" (x-exports wins)
      DB_PASS: "${postgres.password}"
      # Resolves to "real-secret" (direct lookup, no export with this name)
      DB_PASS2: "${postgres.environment.POSTGRES_PASSWORD}"
      # Resolves to "postgres" (virtual field)
      DB_HOST: "${postgres.hostname}"
```

Direct references are most useful for plain image services where duplicating values in `x-exports` is redundant. For Helm services, `x-exports` remains the primary mechanism since service fields are empty in `composed.yaml` (they are generated at build time).

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

## x-shell

`x-shell` is a **top-level** key that runs shell commands on the host during `composed build` and captures their stdout as referenceable values. Shell entries are not services -- they produce no containers.

### Shorthand (string value)

```yaml
x-shell:
  sso-token: "vault kv get -field=token secret/myapp"

services:
  app:
    image: myapp
    environment:
      TOKEN: "${sso-token}"
```

`${sso-token}` resolves to the trimmed stdout of the command.

### Long form (map value)

```yaml
x-shell:
  sso-token:
    command: "vault kv get -field=token secret/myapp"
    allow_failure: true
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `command` | yes | -- | Shell command (passed to `sh -c`), stdout is captured |
| `allow_failure` | no | `false` | If `true`, non-zero exit logs a warning instead of aborting |

### Inline shell reference

For one-off values, use `${shell:...}` directly in any string value:

```yaml
services:
  app:
    image: myapp
    environment:
      TOKEN: "${shell:vault kv get -field=token secret/myapp}"
```

If the same command appears multiple times, it runs multiple times. Use the named form for shared values.

### Execution rules

- All `x-shell` entries run **before** any other processing (Helm rendering, compose merging, reference resolution).
- Named entries run in declaration order.
- Inline `${shell:...}` references are resolved during reference resolution, after named shells have run.
- Stdout is trimmed of leading/trailing whitespace.
- Named shell values share the same reference namespace as `x-exports` -- `${name}` resolves to stdout.

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
