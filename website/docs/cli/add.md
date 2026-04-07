---
sidebar_position: 2
---

# add

```
composed add [name] <source> [flags]
```

Adds a service to `composed.yaml`. Source type is auto-detected:

| Source | Detection |
| ------ | --------- |
| `oci://...` | Probes OCI manifest -- helm chart or container image |
| `*.yaml` / `*.yml` | Compose file include |
| Directory with `Chart.yaml` | Local helm chart |
| `repo/chart` (with `--repo`) | Helm chart repository |
| Everything else | Docker image |

Service name is derived from the source if not given (last path segment, tag stripped).

## Flags

| Flag | Description |
| ---- | ----------- |
| `--set key=val` | Set Helm value (repeatable, supports nested keys) |
| `--values file` | Load values file and merge inline |
| `--values-file path` | Store file reference for build-time loading |
| `--repo url` | Helm chart repository URL |
| `--version constraint` | Chart version constraint |
| `--port host:container` | Port mapping (image type, repeatable) |
| `--env KEY=VAL` | Environment variable (image type, repeatable) |
| `--volume name:/path` | Volume mount (image type, repeatable) |
| `--depends-on name` | Dependency on another service (repeatable) |

## Examples

```bash
# Auto-detected OCI helm chart
composed add oci://docker.litellm.ai/berriai/litellm-helm

# Docker image with options
composed add postgres:15-alpine --port 5432:5432 --env POSTGRES_PASSWORD=secret

# Explicit name + source
composed add litellm oci://docker.litellm.ai/berriai/litellm-helm --set image.tag=main-stable

# With dependency
composed add myapp:latest --depends-on postgres --depends-on redis
```
