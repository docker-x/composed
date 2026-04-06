# LiteLLM + n8n on Shared Postgres

Two services sharing one Postgres instance with separate schemas.

| Service | Port | Schema | Purpose |
|---------|------|--------|---------|
| Postgres | 5432 | — | Shared database |
| LiteLLM | 4000 | `public` | AI proxy (models, spend logs) |
| n8n | 5678 | `n8n` | Workflow automation |

## Usage

```bash
composed up
```

Then open:
- **n8n**: http://localhost:5678
- **LiteLLM**: http://localhost:4000/ui (key: `sk-1234`)

n8n can call LiteLLM at `http://litellm:4000` using an HTTP Request node
or the OpenAI node with a custom base URL.

## How it works

`composed.yaml` uses `x-exports` + `${service.key}` cross-references so
both services share the same Postgres credentials without duplication:

```yaml
postgres:
  x-exports:
    host: postgres
    password: shared-secret
    database: shared

litellm:
  environment:
    DATABASE_URL: "postgresql://${postgres.user}:${postgres.password}@${postgres.host}/${postgres.database}"

n8n:
  environment:
    DB_POSTGRESDB_HOST: "${postgres.host}"
    DB_POSTGRESDB_SCHEMA: n8n    # own schema, shared database
```

LiteLLM uses the default `public` schema. n8n uses the `n8n` schema
(created automatically on first start).

## Files

| File | Source |
|------|--------|
| `composed.yaml` | Hand-written |
| `docker-compose.yaml` | `composed build` (gitignored) |
