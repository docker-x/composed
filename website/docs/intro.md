---
sidebar_position: 1
slug: /
---

# Composed

**Compose anything into a Docker Compose file.**

Composed lets you combine Helm charts, Docker images, and existing compose files into a single `docker-compose.yaml`. No Kubernetes cluster needed.

---

## How It Works

Your config file is a standard Docker Compose file extended with `x-` extensions. Plain services work with `docker compose up` directly -- Composed only processes the extensions to pull in Helm charts and other sources, then produces one unified compose file.

**Helm charts + Docker images + compose files --> one docker-compose.yaml.**

## Quick Start

```sh
composed init
composed add helm/nginx
composed add docker.io/library/redis:7
composed build
composed up
```

## Learn More

- [Getting Started](getting-started/installation.md) -- Install Composed and run your first project.
- [Config File](guide/config-file.md) -- Understand the compose file format and `x-` extensions.
- [CLI Reference](cli/init.md) -- Full reference for every command.

## Part of Docker eXtra

Composed is part of the [Docker eXtra](https://github.com/docker-x) project.
