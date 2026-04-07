---
sidebar_position: 1
---

# Architecture

## Pipeline

The `composed build` command transforms a `composed.yaml` configuration into a
standard `docker-compose.yaml` file. The pipeline executes in a single pass
through several well-defined stages:

```
composed.yaml
     |
     v
+---------------------------------------------------+
|                 composed build                     |
+---------------------------------------------------+
|                                                    |
|  1. Parse config (internal/config)                 |
|     +-- Load composed.yaml, resolve x-exports      |
|                                                    |
|  2. For each x-helm service:                       |
|     +-- Render chart (internal/helm)               |
|     |   +-- helm template -> K8s YAML              |
|     +-- Parse K8s manifests (internal/k8s)         |
|     |   +-- Multi-doc split -> typed objects        |
|     +-- Translate (internal/translate)              |
|         +-- K8s objects -> Compose model            |
|                                                    |
|  3. For each x-compose-file service:               |
|     +-- Parse external compose file                |
|                                                    |
|  4. For each plain image service:                  |
|     +-- Pass through as-is                         |
|                                                    |
|  5. Merge all fragments (internal/merge)           |
|     +-- Union services, volumes, networks, configs |
|                                                    |
|  6. Emit YAML (internal/compose)                   |
|     +-- Compose model -> deterministic YAML        |
|                                                    |
+---------------------------------------------------+
     |
     v
docker-compose.yaml
```

**Stage 1** reads the `composed.yaml` file and resolves any `x-exports`
references. The result is a fully resolved configuration object containing every
service definition.

**Stage 2** handles Helm-based services. Each service with an `x-helm` extension
triggers a chart pull, a `helm template` render, Kubernetes manifest parsing, and
finally translation into the Compose model.

**Stage 3** handles services that reference an external `docker-compose.yaml`
fragment via `x-compose-file`. These are parsed directly into the Compose model.

**Stage 4** handles plain image services (no Helm chart, no external file). These
pass through to the output unchanged.

**Stage 5** merges every Compose fragment produced by the previous stages into a
single unified model. Services, volumes, networks, and configs are unioned
together.

**Stage 6** serializes the merged Compose model into deterministic YAML. Map keys
are sorted before emission to guarantee reproducible output.

## Package Layout

```
composed/
├── main.go                        # entrypoint
├── cmd/                           # CLI wiring (cobra)
│   ├── root.go                    # root command + global flags
│   ├── build.go                   # build subcommand (+ up/down)
│   ├── config.go                  # init + add subcommands
│   ├── oci.go                     # OCI registry manifest probing
│   └── resolve.go                 # config file walk-up resolution
│
├── internal/
│   ├── config/config.go           # Config model (File, Service, HelmExtension)
│   ├── helm/renderer.go           # Helm SDK: pull chart, render templates
│   ├── k8s/parser.go              # Multi-doc YAML → typed K8s objects
│   ├── translate/translate.go     # K8s → Compose translator (the big one)
│   ├── compose/
│   │   ├── model.go               # Typed Compose file model
│   │   └── emit.go                # Model → YAML string (deterministic)
│   └── merge/merge.go             # Merges compose fragments
```

Each package has a single responsibility:

- **cmd/** wires CLI commands to internal logic. No business logic lives here.
- **internal/config** owns the `composed.yaml` schema and parsing.
- **internal/helm** wraps the Helm SDK to fetch and render charts.
- **internal/k8s** splits multi-document YAML streams and decodes them into typed
  Kubernetes API objects.
- **internal/translate** is the core translator that converts Kubernetes objects
  into Compose service definitions.
- **internal/compose** defines the output Compose model and handles YAML
  serialization.
- **internal/merge** combines multiple Compose fragments into a single file.

## Cross-Referencing Strategy

The translator in `internal/translate` uses a multi-pass approach to resolve
references between Kubernetes objects:

1. **Collect** -- Parse all Kubernetes documents from the rendered Helm output
   and bucket them by Kind (Deployment, Service, ConfigMap, Secret, PVC, etc.).

2. **Index** -- Build lookup maps keyed by resource name: ConfigMaps by name,
   Secrets by name, PVCs by name, and Services by selector labels. These maps
   enable O(1) lookups during translation.

3. **Translate workloads** -- For each Deployment or StatefulSet, walk its pod
   spec and:
   - Resolve `envFrom` and `env.valueFrom` references against the ConfigMap and
     Secret indexes.
   - Resolve `volumeMounts` against the PVC index to produce Compose volume
     definitions.
   - Create `depends_on` entries for init containers that map to other services.

4. **Apply ports** -- For each Kubernetes Service, find the Compose service whose
   labels match the Service's selector and attach the corresponding port
   mappings (containerPort -> published port).

5. **Collect orphans** -- ConfigMaps, Secrets, and PVCs that were not referenced
   by any workload during step 3 generate warnings. These orphans may indicate
   chart resources that have no Compose equivalent or a gap in the translator.

## Dependencies

| Package | Purpose |
|---------|---------|
| `helm.sh/helm/v3` | Chart fetch + template rendering |
| `k8s.io/api` | Typed Kubernetes resource structs |
| `k8s.io/apimachinery` | Runtime object decoding |
| `sigs.k8s.io/yaml` | Kubernetes-flavored YAML parsing |
| `gopkg.in/yaml.v3` | Compose YAML output (ordered maps) |
| `github.com/spf13/cobra` | CLI framework |

The Helm SDK (`helm.sh/helm/v3`) is the heaviest dependency. It pulls in a
significant portion of the Kubernetes client libraries transitively. The
`k8s.io/api` and `k8s.io/apimachinery` packages provide the typed structs and
decoder needed to work with rendered manifests without running a cluster.

## Design Principles

- **Internal packages under `internal/`** -- No package under `internal/` is
  importable by external code. This boundary enforces that the public API surface
  is limited to the CLI itself.

- **Deterministic output** -- All maps are sorted by key before emitting YAML.
  Running `composed build` twice on the same input always produces byte-identical
  output. This makes diffs meaningful and CI caching reliable.

- **Minimal framework surface** -- The project uses only stdlib, cobra, yaml.v3,
  and the k8s.io types. There are no ORM layers, DI containers, or plugin
  systems.

- **Spec-driven translation** -- The translation logic follows the rules defined
  in `DESIGN.md`. Any code behavior that contradicts the spec is treated as a
  bug, not a feature.
