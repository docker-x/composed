# Config File

The `composed.yaml` file is the single source of truth for your project. It declares every service -- Helm charts, Docker images, and external compose files -- in one place.

## Format

`composed.yaml` is a **valid Docker Compose file** extended with `x-` prefixed fields. The Docker Compose specification treats any top-level or service-level key starting with `x-` as an extension and ignores it. This means:

- Services that only use standard fields (image, ports, environment, etc.) work with `docker compose up` directly -- no build step needed.
- Services that use `x-helm` or `x-compose-file` need `composed build` to produce the final `docker-compose.yaml`.

You can always run `docker compose config -f composed.yaml` to validate the file syntax, even if it contains extensions.

## Service types

Composed infers the service type from which extensions are present:

| Has | Type | Behavior |
|-----|------|----------|
| `x-helm` | helm | Chart rendered via `helm template`, Kubernetes manifests translated to compose services |
| `x-compose-file` | compose | External compose file parsed and merged into the output |
| (neither) | image | Standard compose service, passed through as-is |

A service can only be one type. If both `x-helm` and `x-compose-file` are set, `x-helm` takes precedence.

## Config file resolution

All commands (`build`, `up`, `down`, `add`) walk up from the current working directory to find `composed.yaml`, similar to how `git` walks up to find `.git/`. This lets you run commands from any subdirectory in your project.

If no `composed.yaml` is found in any parent directory, the command looks in the current directory.

Use the `-f` flag to override automatic discovery and point to a specific file:

```sh
composed build -f path/to/my-config.yaml
```

## Full example

Below is a complete `composed.yaml` that uses all three service types:

```yaml
name: my-stack

services:
  # --- Helm chart service ---
  redis:
    x-helm:
      chart: bitnami/redis
      repo: https://charts.bitnami.com/bitnami
      version: "18.x"
      values:
        architecture: standalone
        auth:
          enabled: false
    x-exports:
      host: redis-master
      port: "6379"

  # --- External compose file ---
  monitoring:
    x-compose-file: ./monitoring/docker-compose.yaml

  # --- Plain image service ---
  postgres:
    image: postgres:15-alpine
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: myapp
      POSTGRES_PASSWORD: secret
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 3s
      retries: 5
    restart: unless-stopped
    x-exports:
      host: postgres
      port: "5432"
      password: secret

  # --- Plain image service with cross-references ---
  app:
    image: my-app:latest
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: "postgresql://postgres:${postgres.password}@${postgres.host}:${postgres.port}/myapp"
      REDIS_URL: "redis://${redis.host}:${redis.port}"
    depends_on:
      - postgres
      - redis
    restart: unless-stopped

volumes:
  pgdata:
```

Running `composed build` on this file will:

1. Render the `bitnami/redis` Helm chart and translate the Kubernetes manifests into compose services.
2. Parse `./monitoring/docker-compose.yaml` and merge its services, volumes, and networks into the output.
3. Pass the `postgres` and `app` services through as-is.
4. Resolve `${postgres.host}`, `${postgres.password}`, `${redis.host}`, etc. using each service's `x-exports`.
5. Write the merged result to `docker-compose.yaml`.
