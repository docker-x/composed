# init

```
composed init [--project <name>] [--helm-values]
```

Creates a new `composed.yaml` in the current directory.

## Flags

| Flag | Description |
| ---- | ----------- |
| `--project <name>` | Set the project name. Defaults to the current directory name. |
| `--helm-values` | Scaffold values files for all helm services. Runs `helm show values <chart>` for each service with `x-helm`, writes the output to a values file, and adds the `values_file` reference to `composed.yaml`. Idempotent -- skips services that already have a values file. |

## Examples

```bash
composed init                        # project name = directory name
composed init --project my-stack     # explicit project name
composed init --helm-values          # scaffold values files for helm services
```
