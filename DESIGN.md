# composed — Design Document

## Problem

Helm is the de-facto package manager for Kubernetes. Thousands of production-grade
charts exist for databases, queues, monitoring stacks, and more. But if you're
running Docker Desktop locally (no K8s cluster), those charts are unusable.

**composed** bridges this gap: it renders a Helm chart and translates the
Kubernetes manifests into a Docker Compose file that runs on plain Docker.

## Non-Goals

- Full K8s fidelity. Operators, CRDs, RBAC, NetworkPolicy, HPA, Ingress
  controllers are **out of scope** — they don't have Compose equivalents and
  don't make sense for local development.
- Running a Kubernetes API server. This is a static translation, not an emulator.
- Helm lifecycle management (install/upgrade/rollback). This is a one-shot
  renderer, not Tiller.

## CLI Interface

```
composed init [--project <name>]                              # create composed.yaml
composed add [name] <source> [flags]                          # add service (auto-detects type)
composed build [-f composed.yaml] [-o docker-compose.yaml]    # build compose from config
composed up [-f composed.yaml]                                # build + docker compose up
composed down                                                 # docker compose down
```

### `init` — create a project

```bash
composed init                        # project name = directory name
composed init --project my-stack     # explicit name
composed init --helm-values          # scaffold values files for helm services
```

`--helm-values` scans `composed.yaml` for services with `x-helm`, runs
`helm show values <chart>` for each, writes `values-<name>.yaml`, and adds
the `values_file:` reference. Idempotent — skips services that already have
a values file.

### `add` — add a service

Source type is auto-detected:
- `oci://...` → probes OCI registry manifest (`config.mediaType`) to distinguish helm charts from images
- `*.yaml` / `*.yml` file → compose file include
- Directory with `Chart.yaml` → local helm chart
- `repo/chart` (with `--repo`) → helm chart repository
- Everything else → Docker image

Service name is derived from the source if not given (last path segment, tag stripped).

```bash
# Fully automatic — name and type from OCI manifest
composed add oci://docker.litellm.ai/berriai/litellm-helm

# Docker image
composed add postgres:15-alpine --port 5432:5432 --env POSTGRES_PASSWORD=secret

# Explicit name + source
composed add litellm oci://docker.litellm.ai/berriai/litellm-helm --set image.tag=main-stable
```

#### Helm values (3 ways)

| Method | Flag | Behavior |
|--------|------|----------|
| Inline | `--set key=val` | Stored in `x-helm.values:` in composed.yaml |
| Merge file | `--values file.yaml` | File contents merged inline into composed.yaml at add time |
| Reference | `--values-file ./file.yaml` | Path stored as `x-helm.values_file:`, loaded at build time |

Merge priority (low → high): `values_file` → inline `values:` → `--set`

| Flag | Description |
|------|-------------|
| `--set <key=val>` | Set Helm value (repeatable, supports nested keys like `image.tag=v1`) |
| `--values <file>` | Load values file and merge inline |
| `--values-file <path>` | Store file reference for build-time loading |
| `--repo <url>` | Helm chart repository URL |
| `--version <constraint>` | Chart version constraint |
| `--port <host:container>` | Port mapping (image type, repeatable) |
| `--env <KEY=VAL>` | Environment variable (image type, repeatable) |
| `--volume <name:/path>` | Volume mount (image type, repeatable) |
| `--depends-on <name>` | Dependency on another service (repeatable) |

### `build` — full pipeline

```bash
composed build                                   # finds composed.yaml walking up from cwd
composed build -f composed.yaml -o output.yaml   # explicit paths
composed build -o -                              # stdout
```

### `up` — build and start

```bash
composed up
```

### `down` — stop the stack

```bash
composed down
```

### Config file resolution

All commands (`build`, `up`, `add`) walk up the directory tree from cwd to find
`composed.yaml`, like `git` finds `.git`. The `-f` flag overrides this.

## composed.yaml — Compose with Extensions

`composed.yaml` is a valid Docker Compose file extended with `x-` prefixed
fields ([Docker Compose extension mechanism](https://docs.docker.com/reference/compose-file/extension/)).
Docker Compose ignores `x-` fields, so plain image services work with
`docker compose up` directly. Services with `x-helm` or `x-compose-file` need
`composed build` to resolve into real services.

### Service types (inferred from extensions)

A service's type is determined by which `x-` extension it has:

| Has | Type | Behavior |
|-----|------|----------|
| `x-helm` | helm | Chart is rendered via `helm template`, K8s manifests are translated to compose |
| `x-compose-file` | compose | External compose file is parsed and merged into output |
| (neither) | image | Standard compose service, passed through as-is |

### `x-helm` — Helm chart rendering

```yaml
services:
  litellm:
    x-helm:
      chart: oci://docker.litellm.ai/berriai/litellm-helm   # OCI ref, repo/name, or local path
      repo: https://charts.bitnami.com/bitnami               # Chart repository URL (optional)
      version: "1.82.3"                                       # Chart version constraint (optional)
      values:                                                  # Inline values (passed as --set)
        image:
          tag: main-stable
      values_file: ./redis-values.yaml                         # Values file (passed as -f to helm)
```

### `x-compose-file` — Include external compose file

```yaml
services:
  monitoring:
    x-compose-file: ./monitoring/docker-compose.yaml
```

### `x-exports` — Cross-service references

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
    depends_on:
      - postgres
```

Other services reference exports via `${service_name.key}`. These are resolved
by `composed build` before any Helm rendering or output.

### Direct references

In addition to `x-exports`, services can reference fields from other services
directly using path syntax:

```text
${service_name.environment.KEY}   # value of environment variable KEY
${service_name.hostname}          # always resolves to the service name (Compose DNS)
${service_name.image}             # the image field
${service_name.ports[N]}          # Nth port mapping (e.g. "5432:5432")
```

Direct references and `x-exports` can be mixed. Resolution priority:
1. `x-exports` (checked first — explicit interface wins)
2. Direct field lookup (fallback)

This means `x-exports` can override or alias a direct field:

```yaml
services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_PASSWORD: secret
    x-exports:
      password: secret       # explicit export

  app:
    environment:
      # Both styles work:
      DB_PASS: "${postgres.password}"                    # from x-exports
      DB_PASS2: "${postgres.environment.POSTGRES_PASSWORD}"  # direct reference
      DB_HOST: "${postgres.hostname}"                    # always = "postgres"
```

Direct references are most useful for plain image services where duplicating
values in `x-exports` is redundant. For Helm services, `x-exports` remains
the primary mechanism since the service fields are empty in `composed.yaml`
(they're generated at build time).

### Standard compose fields

All standard Docker Compose service fields work as-is on any service:
`image`, `environment`, `ports`, `volumes`, `command`, `entrypoint`,
`healthcheck`, `labels`, `depends_on`, `restart`, etc.

## Architecture

```
┌─────────────┐     ┌──────────┐     ┌────────────┐     ┌─────────┐
│  Helm SDK   │────>│  Parser  │────>│ Translator │────>│ Emitter │──> compose YAML
│ (fetch+tmpl)│     │ (multi-  │     │ (k8s → IR) │     │ (IR →   │
│             │     │  doc)    │     │            │     │  YAML)  │
└─────────────┘     └──────────┘     └────────────┘     └─────────┘
      ▲                  ▲                                    │
      │                  │                                    │
  render cmd         convert cmd                          stdout / -o
```

### Package Layout

```
composed/
├── main.go                     # entrypoint
├── go.mod
├── go.sum
├── DESIGN.md                   # this file
├── README.md
│
├── cmd/                        # CLI wiring (cobra)
│   ├── root.go                 # root command + global flags
│   ├── build.go                # build subcommand (+ up/down)
│   ├── config.go               # init + add subcommands
│   ├── oci.go                  # OCI registry manifest probing
│   └── resolve.go              # config file walk-up resolution
│
├── internal/
│   ├── config/
│   │   └── config.go           # Config model (File, Service, HelmExtension, etc.)
│   │
│   ├── helm/
│   │   └── renderer.go         # Helm SDK: pull chart, render templates
│   │
│   ├── k8s/
│   │   └── parser.go           # Multi-doc YAML → []k8s.Object
│   │
│   ├── translate/
│   │   └── translate.go        # K8s → Compose translator
│   │
│   ├── compose/
│   │   ├── model.go            # Typed Compose file model
│   │   └── emit.go             # Model → YAML string
│   │
│   └── merge/
│       └── merge.go            # Merges compose fragments
│
└── testdata/                   # Test fixtures
    ├── redis-standalone/       # helm template output + expected compose
    ├── postgres/
    └── multi-service/
```

## Translation Rules

### Workloads → Compose Services

**Deployment / StatefulSet / DaemonSet** each produce one compose service per
container in `spec.template.spec.containers[]`:

```
K8s Deployment "redis-master"                Compose service "redis-master"
├── .spec.replicas                      →    deploy.replicas
├── .spec.template.spec.containers[0]
│   ├── .image                          →    image
│   ├── .command                        →    entrypoint
│   ├── .args                           →    command
│   ├── .env[]                          →    environment (inline values)
│   ├── .env[].valueFrom.configMapKeyRef →   environment (resolved from ConfigMap)
│   ├── .env[].valueFrom.secretKeyRef   →    environment (resolved from Secret)
│   ├── .envFrom[].configMapRef         →    environment (bulk merge from ConfigMap.data)
│   ├── .envFrom[].secretRef            →    environment (bulk merge from Secret.data)
│   ├── .ports[].containerPort          →    (used by Service translation for mapping)
│   ├── .volumeMounts[]                 →    volumes (cross-ref with PVC or ConfigMap)
│   ├── .resources.limits               →    deploy.resources.limits
│   ├── .resources.requests             →    deploy.resources.reservations
│   ├── .livenessProbe                  →    healthcheck
│   └── .readinessProbe                 →    healthcheck (fallback if no liveness)
└── .spec.template.spec.initContainers[]→    separate service + depends_on chain
```

Multi-container pods: each sidecar becomes `<deployment>-<container-name>`.

### K8s Service → Port Mappings

The translator matches a K8s Service's `.spec.selector` to a Deployment's
`.spec.template.metadata.labels`. Matching rules:

| K8s Service Type | Compose Mapping |
|------------------|-----------------|
| ClusterIP | No port mapping. Compose DNS handles inter-service resolution. |
| NodePort | `nodePort:targetPort` on the matched compose service |
| LoadBalancer | `port:targetPort` on the matched compose service |

If a Service has no matching Deployment (e.g. headless services for StatefulSets),
the ports go on the StatefulSet's compose service.

### ConfigMap → Environment or Config

| Usage Pattern | Compose Mapping |
|---------------|-----------------|
| `envFrom: configMapRef` | Merge all `.data` keys into `service.environment` |
| `env[].valueFrom.configMapKeyRef` | Single key into `service.environment` |
| `volumeMount` referencing a ConfigMap | Compose `configs:` top-level + service config mount |
| Unreferenced | Skipped with warning |

### Secret → Environment

Same as ConfigMap, but `.data` values are base64-decoded. A warning is emitted
that secrets will appear as plaintext in the compose file.

### PersistentVolumeClaim → Named Volume

```
K8s PVC "redis-data"          Compose volume "redis-data"
├── .metadata.name        →   volume name
└── .spec.resources        →   (informational comment, compose doesn't enforce)
```

Volume mounts cross-reference: if a container's `volumeMount` references a
volume with `persistentVolumeClaim.claimName`, the compose service gets
`redis-data:/data/mountPath`.

### Init Containers → depends_on Chain

Each init container becomes a separate compose service:

```yaml
services:
  redis-master-init-sysctl:
    image: bitnami/os-shell:12
    entrypoint: ["/bin/sh", "-c", "sysctl -w net.core.somaxconn=65535"]
    deploy:
      restart_policy:
        condition: on-failure
        max_attempts: 3
  redis-master:
    depends_on:
      redis-master-init-sysctl:
        condition: service_completed_successfully
```

### Job → One-Shot Service

```yaml
services:
  db-migrate:
    image: my-app:latest
    command: ["rake", "db:migrate"]
    deploy:
      restart_policy:
        condition: on-failure
        max_attempts: 3
    # No `ports:` — jobs are not long-running
```

### Probe → Healthcheck

```
K8s livenessProbe (preferred) or readinessProbe:
├── exec.command            →  healthcheck.test: ["CMD", ...command]
├── httpGet                 →  healthcheck.test: ["CMD", "wget", "-q", "--spider", "http://localhost:port/path"]
├── tcpSocket               →  healthcheck.test: ["CMD", "sh", "-c", "cat < /dev/tcp/localhost/port"]
├── periodSeconds           →  healthcheck.interval
├── timeoutSeconds          →  healthcheck.timeout
├── failureThreshold        →  healthcheck.retries
└── initialDelaySeconds     →  healthcheck.start_period
```

### Resource Limits

```
K8s resources:                    Compose deploy.resources:
├── limits.memory: 256Mi     →   limits.memory: 256M
├── limits.cpu: "500m"       →   limits.cpus: "0.5"
├── requests.memory: 128Mi   →   reservations.memory: 128M
└── requests.cpu: "250m"     →   reservations.cpus: "0.25"
```

### Skipped Resources (with warnings)

| Kind | Reason |
|------|--------|
| ServiceAccount | No Compose equivalent |
| ClusterRole / ClusterRoleBinding | No Compose equivalent |
| Role / RoleBinding | No Compose equivalent |
| NetworkPolicy | No Compose equivalent |
| Ingress | Would need a reverse proxy; out of scope |
| HorizontalPodAutoscaler | No Compose equivalent |
| PodDisruptionBudget | No Compose equivalent |
| CustomResourceDefinition | Opaque; can't translate |
| Any unknown CRD instance | Opaque; can't translate |

## Cross-Referencing Strategy

The translator works in passes:

1. **Collect** — parse all documents, bucket by Kind
2. **Index** — build lookup maps:
   - ConfigMaps by name
   - Secrets by name
   - PVCs by name
   - Services by selector labels
3. **Translate workloads** — for each Deployment/StatefulSet:
   - Resolve env refs → look up ConfigMap/Secret by name
   - Resolve volume mounts → look up PVC by claim name
   - Resolve init containers → create depends_on services
4. **Apply ports** — for each K8s Service, find the compose service with
   matching labels and attach port mappings
5. **Collect orphans** — ConfigMaps/Secrets/PVCs not referenced by any
   workload get warnings

## Dependencies

```
helm.sh/helm/v3         # Chart fetch + template rendering
k8s.io/api              # Typed K8s resource structs
k8s.io/apimachinery     # Runtime object decoding, YAML/JSON utilities
sigs.k8s.io/yaml        # K8s-flavored YAML (JSON superset)
gopkg.in/yaml.v3        # Compose YAML output (ordered maps)
github.com/spf13/cobra  # CLI framework
```

## Output Example

Input: `bitnami/redis` with `architecture=standalone`

```yaml
# Generated by composed from bitnami/redis 18.6.1
# Translated: 1 Deployment, 1 Service, 1 ConfigMap, 1 Secret, 1 PVC
# Skipped: 2 ServiceAccount, 1 NetworkPolicy

services:
  redis-master:
    image: docker.io/bitnami/redis:7.2.4-debian-12-r9
    command: ["/opt/bitnami/scripts/redis/run.sh"]
    environment:
      REDIS_PASSWORD: "secret"
      REDIS_PORT: "6379"
      REDIS_AOF_ENABLED: "yes"
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/bitnami/redis/data
    healthcheck:
      test: ["CMD", "redis-cli", "--no-auth-warning", "-a", "$$REDIS_PASSWORD", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    deploy:
      resources:
        limits:
          memory: 256M

volumes:
  redis-data:
```

## Open Questions

1. **Network isolation** — Should the tool create a dedicated compose network per
   chart, or use the default? Current decision: use the default network (simplest).
   Add `--network <name>` flag if users want isolation.

2. **Image registry rewriting** — Some charts use private registries. Should we
   support `--registry-mirror` for local development? Deferred.

3. **Helm hooks** — Pre-install/post-install hooks are Jobs with annotations.
   Should we translate them to one-shot services with `profiles:` so they don't
   run by default? Worth considering.
