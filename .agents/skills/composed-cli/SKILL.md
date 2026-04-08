---
name: composed-cli
description: Build and use the composed CLI to combine Helm charts, Docker images, and compose files into a single docker-compose.yaml. Use when managing stacks defined in composed.yaml, converting Helm charts to Compose, or scaffolding new projects with init/add.
---

# composed

## When to Use

- User wants to compose a multi-service stack from Helm charts, existing compose files, and/or plain images
- User asks to "build", "deploy", or "bring up" a stack defined in a `composed.yaml`
- User wants to convert Kubernetes manifests or Helm charts to Docker Compose
- User mentions composed, composed.yaml, or "mega compose"

## Binary Location

```
~/.local/bin/composed
```

Source: https://github.com/docker-x/composed

If the binary doesn't exist or is stale, build it:
```bash
go install github.com/docker-x/composed@latest
```

Or from source:
```bash
git clone https://github.com/docker-x/composed.git
cd composed && go build -o composed . && ln -sf "$(pwd)/composed" ~/.local/bin/composed
```

## Commands

### `init` — Create a new project

```bash
composed init                        # project name = directory name
composed init --project my-stack
```

### `init --helm-values` — Scaffold values files

```bash
composed init --helm-values
```

Scans `composed.yaml` for services with `x-helm`, runs `helm show values` for
each, and writes `<name>.values.yaml`. The file is identical to what
`helm show values` returns — all options with comments. The `values_file:`
reference is added to `x-helm` automatically.

Idempotent — skips services that already have a `values_file`.

### `add` — Add a service (auto-detects type)

Source type is auto-detected by probing OCI registry manifests or inspecting the filesystem:
- `oci://...` → probes registry manifest (helm chart or container image)
- `*.yaml` / `*.yml` → compose file include
- Directory with `Chart.yaml` → local helm chart
- Directory with K8s YAML files (containing `kind:` and `apiVersion:`) → K8s manifests
- Everything else → Docker image

Name is derived from the source if not given.

```bash
# Fully automatic
composed add oci://docker.litellm.ai/berriai/litellm-helm
composed add postgres:15-alpine --port 5432:5432 --env POSTGRES_PASSWORD=secret

# Explicit name
composed add litellm oci://docker.litellm.ai/berriai/litellm-helm --set image.tag=main-stable

# Local chart directory
composed add ./litellm-helm
composed add ./litellm-helm --values-file litellm-helm.values.yaml

# Helm values (3 ways)
composed add oci://... --set key=val                    # inline in composed.yaml
composed add oci://... --values values.yaml             # merge file inline
composed add oci://... --values-file ./values.yaml      # reference, loaded at build time

# With dependencies
composed add myapp:latest --depends-on postgres --depends-on redis

# K8s manifests directory (auto-detected)
composed add ./k8s/manifests

# K8s with explicit flag
composed add my-app --k8s-path ./k8s/manifests
```

### `build` — Build docker-compose.yaml

```bash
composed build                    # finds composed.yaml walking up from cwd
composed build -f composed.yaml -o docker-compose.yaml
composed build -o -               # stdout
```

### `up` — Build and start

```bash
composed up
```

### `down` — Stop the stack

```bash
composed down
```

## composed.yaml Format

`composed.yaml` is a Docker Compose file with `x-` extensions. Plain services
work with `docker compose up` directly. Files with `x-helm`, `x-k8s`, `x-compose-file`,
or `x-shell` entries need `composed build` first.

```yaml
name: my-stack

services:
  # Plain compose service — standard syntax, zero new concepts
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_PASSWORD: secret
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready"]
    x-exports:
      host: postgres
      password: secret

  # Helm chart — x-helm extension
  litellm:
    x-helm:
      chart: oci://docker.litellm.ai/berriai/litellm-helm
      values_file: ./values.yaml      # loaded at build time via helm -f
      values:                          # inline overrides (passed via --set)
        image:
          tag: main-stable

  # Include external compose file
  monitoring:
    x-compose-file: ./monitoring/docker-compose.yaml

  # K8s manifests — cdk8s, kustomize, hand-written YAML, etc.
  my-app:
    x-k8s:
      command: "cdk8s synth"         # optional pre-build command
      path: ./my-cdk8s-app/dist      # directory or single YAML file

  # Cross-service references via x-exports
  app:
    image: my-app:latest
    environment:
      DATABASE_URL: "postgresql://postgres:${postgres.password}@${postgres.host}/mydb"
    depends_on:
      - postgres
```

## Cross-service references

Two ways to reference values from other services:

### x-exports (explicit interface)

Define key-value pairs on a service, reference them as `${service.key}`:

```yaml
services:
  postgres:
    image: postgres:15
    x-exports:
      host: postgres
      password: secret

  app:
    environment:
      DB_URL: "postgresql://postgres:${postgres.password}@${postgres.host}/mydb"
```

### Direct references (field lookup)

Reference service fields directly without exports:

| Syntax | Resolves to |
|--------|-------------|
| `${svc.environment.KEY}` | Value of env var KEY |
| `${svc.hostname}` | Service name (Compose DNS) |
| `${svc.image}` | The image field |
| `${svc.ports[N]}` | Nth port mapping (0-indexed) |

```yaml
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: secret

  app:
    environment:
      DB_PASS: "${postgres.environment.POSTGRES_PASSWORD}"
      DB_HOST: "${postgres.hostname}"
```

**Priority**: x-exports checked first (explicit wins), then direct field lookup.
Best for plain image services. For Helm services, use x-exports (fields are empty until build).

## x-shell (host commands during build)

Top-level key that runs shell commands and captures stdout as referenceable values.
Three syntax tiers:

```yaml
# 1. Named shorthand — stdout becomes ${name}
x-shell:
  sso-token: "vault kv get -field=token secret/myapp"

# 2. Named long form — with options
x-shell:
  sso-token:
    command: "vault kv get -field=token secret/myapp"
    allow_failure: true

# 3. Inline — one-off, no name needed
services:
  app:
    environment:
      TOKEN: "${shell:vault kv get -field=token secret/myapp}"
```

Shell entries run before all other processing. Named values share the reference namespace with x-exports.

## Component pattern (x-compose-file)

Components should be generic infrastructure (no hardcoded credentials). The
consumer provides credentials via `environment:` or `env_file:` in composed.yaml.

```yaml
# pgvector/compose.yaml — reusable, no credentials
services:
  postgres:
    image: pgvector/pgvector:pg15
    ports: ["5432:5432"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready"]
```

```yaml
# composed.yaml — consumer provides credentials
services:
  postgres:
    x-compose-file: ./pgvector/compose.yaml
    env_file:
      - ./postgres.env     # POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB

  app:
    image: my-app:latest
    environment:
      DATABASE_URL: "postgresql://${postgres.environment.POSTGRES_USER}:${postgres.environment.POSTGRES_PASSWORD}@${postgres.hostname}/mydb"
```

Cross-refs resolve from: inline `environment:` > `env_file:` entries > component
`environment:` > component `env_file:`. Preloaded values are for resolution only
— they don't leak into the output.

## Values merge priority

`values_file` (helm `-f`, base) → inline `values:` (helm `--set`, highest)

## Config file resolution

All commands walk up the directory tree to find `composed.yaml` (like `git` finds `.git`). Use `-f` to override.

## Workflows

### Quick start (OCI chart)

```bash
composed init
composed add oci://docker.litellm.ai/berriai/litellm-helm
composed up
```

### With full values reference

```bash
composed init
composed add oci://docker.litellm.ai/berriai/litellm-helm
composed init --helm-values    # writes litellm-helm.values.yaml
# edit litellm-helm.values.yaml
composed up
```

### Local chart (fastest iteration)

```bash
composed init
helm pull oci://docker.litellm.ai/berriai/litellm-helm --untar
composed add ./litellm-helm --values-file litellm-helm.values.yaml
# write litellm-helm.values.yaml with just your overrides
composed up
```

No network after initial pull. Edit values or even chart templates, then `composed up`.

### K8s manifests (cdk8s, kustomize, etc.)

```bash
composed init
cdk8s init                           # or write K8s YAML by hand
cdk8s synth                          # generates dist/
composed add ./dist                  # auto-detected as K8s manifests
composed up
```

Or with the command integrated:

```bash
composed init
# Manually add x-k8s with command to composed.yaml:
# services:
#   my-app:
#     x-k8s:
#       command: "cdk8s synth"
#       path: ./dist
composed up                          # runs cdk8s synth, then translates
```

### What to track in git

Only `composed.yaml`, values files, and READMEs. Everything else is reproduced:

```gitignore
# Downloaded charts
litellm-helm/

# Generated output
docker-compose.yaml
```

## Key Files

| File | Purpose |
|------|---------|
| `main.go` | CLI entrypoint |
| `cmd/config.go` | init + add + init --helm-values |
| `cmd/build.go` | build + up + down commands |
| `cmd/oci.go` | OCI registry manifest detection |
| `cmd/resolve.go` | Config file walk-up resolution |
| `internal/config/` | composed.yaml parsing |
| `internal/helm/` | Helm chart rendering |
| `internal/k8s/` | K8s manifest parser |
| `internal/translate/` | K8s-to-Compose translator |
| `internal/compose/` | Compose model + YAML emitter |
| `internal/merge/` | Multi-compose merger |

## Container Labels

Every service in the output `docker-compose.yaml` is stamped with labels:

```yaml
labels:
  com.composed.managed: "true"
  com.composed.project: my-stack
```

Find composed-managed containers:
```bash
docker ps --filter label=com.composed.managed
docker ps --filter label=com.composed.project=my-stack
```

## Examples

| Example | Description |
|---------|-------------|
| `examples/litellm-chart/` | Minimal — OCI chart, 3 commands |
| `examples/litellm-chart-with-values/` | OCI chart + scaffolded values file |
| `examples/litellm-chart-local/` | Local chart + hand-written overrides |
| `examples/litellm-n8n-shared-pg/` | Shared Postgres, cross-refs via x-exports |
