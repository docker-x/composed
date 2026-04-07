---
slug: /
sidebar_position: 0
---

# Composed

**Run Helm charts and Docker images together with `docker compose` -- no Kubernetes required.**

Composed renders Helm charts into Docker Compose services, wires them together with other containers, and outputs a single standard `docker-compose.yaml` you can inspect, edit, and run anywhere.

## Why Composed?

You want to run a Helm chart locally -- maybe an API gateway, a database operator, or a monitoring stack -- alongside a few plain Docker images. Today that means either spinning up a full K8s cluster (minikube, kind) or manually translating YAML. Composed does this automatically:

```yaml
# composed.yaml -- this IS a valid Docker Compose file
services:
  litellm:
    x-helm:
      chart: oci://ghcr.io/berriai/litellm-helm
    environment:
      DATABASE_URL: "postgresql://admin:${postgres.password}@${postgres.host}:5432/litellm"

  postgres:
    image: postgres:16
    x-exports:
      host: postgres
      password: secret
```

```sh
composed build   # renders Helm chart, translates K8s → Compose, resolves references
composed up      # runs docker compose up
```

## Features

### Helm charts as Compose services

Point at any Helm chart -- OCI registry, repo URL, or local directory. Composed renders it with `helm template`, translates the K8s manifests (Deployments, StatefulSets, Services, ConfigMaps, Secrets, PVCs, Jobs) into Compose services, and wires up ports, volumes, healthchecks, and environment variables.

### Cross-service references

Reference values from one service in another using `${service.key}` placeholders -- like Terraform variables but for Compose. Define exports on any service, use them anywhere.

```yaml
services:
  redis:
    image: redis:7
    x-exports:
      host: redis
      port: "6379"

  app:
    image: myapp:latest
    environment:
      REDIS_URL: "redis://${redis.host}:${redis.port}"
```

### Mix anything

Combine three source types in one file:

| Source | Extension | Example |
|--------|-----------|---------|
| Helm chart | `x-helm` | `chart: oci://ghcr.io/org/chart` |
| Compose file | `x-compose-file` | `file: ./other/docker-compose.yaml` |
| Docker image | *(none)* | `image: postgres:16` |

Services without `x-` extensions pass through unchanged -- your `composed.yaml` works with `docker compose up` directly for plain images.

### Flexible Helm values

Three ways to configure charts, with clear merge priority:

```yaml
services:
  nginx:
    x-helm:
      chart: oci://registry-1.docker.io/bitnamicharts/nginx
      version: "18.1.0"
      values_file: nginx.values.yaml    # full defaults (lowest priority)
      values:                            # inline overrides
        service:
          type: ClusterIP
```

Run `composed init --helm-values` to scaffold default value files for every chart in your stack.

### Smart auto-detection

`composed add` figures out what you're pointing at:

```sh
composed add oci://ghcr.io/org/chart    # OCI Helm chart
composed add docker.io/library/redis:7   # Docker image
composed add ./local-chart/              # local chart directory
composed add bitnami/nginx               # repo/chart reference
```

### Standard output, no lock-in

The generated `docker-compose.yaml` is plain Docker Compose -- no custom runtime, no extensions. Inspect it, commit it, edit it by hand.

## Quick Start

```sh
brew install docker-x/tap/composed   # or: go install github.com/docker-x/composed@latest

composed init                              # create composed.yaml
composed add oci://ghcr.io/berriai/litellm-helm   # add a Helm chart
composed add docker.io/library/redis:7     # add a Docker image
composed build                             # render & translate
composed up                                # docker compose up
```

## Learn More

- [Installation](getting-started/installation.md) -- Install options: Homebrew, binary, Go.
- [Quick Start](getting-started/quick-start.md) -- Full walkthrough with a real Helm chart.
- [Config File](guide/config-file.md) -- Format, service types, build pipeline.
- [Extensions](guide/extensions.md) -- `x-helm`, `x-compose-file`, `x-exports` reference.
- [Translation Rules](guide/translation-rules.md) -- How K8s resources map to Compose.
- [CLI Reference](cli/init.md) -- Every command and flag.

## Part of Docker eXtra

Composed is part of the [Docker eXtra](https://github.com/docker-x) project.
