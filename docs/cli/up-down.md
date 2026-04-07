# up / down

## up

```
composed up [-f composed.yaml]
```

Shortcut: runs `composed build` then `docker compose up -d` on the output.

## down

```
composed down
```

Runs `docker compose down` using the generated `docker-compose.yaml`.

## Examples

```bash
composed up          # build + start in background
composed down        # stop and remove containers
```
