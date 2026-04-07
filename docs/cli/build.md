# build

```
composed build [-f composed.yaml] [-o docker-compose.yaml]
```

Runs the full pipeline: resolves `x-exports`, renders `x-helm` charts (`helm template`), translates K8s manifests to Compose, merges `x-compose-file` includes, and outputs a clean `docker-compose.yaml`.

## Flags

| Flag | Description |
| ---- | ----------- |
| `-f <file>` | Path to `composed.yaml`. Default: walks up from cwd to find it. |
| `-o <file>` | Output path. Default: `docker-compose.yaml`. Use `-o -` for stdout. |

## Config file resolution

If `-f` is not set, composed walks up the directory tree from the current working directory to find `composed.yaml`, similar to how git finds `.git`.

## Examples

```bash
composed build                                   # finds composed.yaml, outputs docker-compose.yaml
composed build -f composed.yaml -o output.yaml   # explicit paths
composed build -o -                              # stdout
```
