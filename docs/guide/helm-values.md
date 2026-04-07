# Helm Values

There are three ways to pass values to a Helm chart service in Composed. Each method targets a different workflow: quick one-off overrides, importing a complete values file, or keeping values in a separate file that is loaded at build time.

## Methods

| Method | Flag | Behavior |
|--------|------|----------|
| Inline | `--set key=val` | Stored in `x-helm.values:` in `composed.yaml` |
| Merge file | `--values file.yaml` | File contents merged inline into `composed.yaml` at add time |
| Reference | `--values-file ./file.yaml` | Path stored as `x-helm.values_file:`, loaded at build time |

## Merge priority

When multiple methods are used together, values are merged from lowest to highest priority:

```
values_file  ->  inline values:  ->  --set
(lowest)                            (highest)
```

At build time, Composed loads the `values_file` first, then applies inline `values:` on top, with `--set` overrides winning last. This matches Helm's own precedence behavior.

---

## Inline values with --set

Use `--set` to store individual key-value pairs directly in `composed.yaml`. The flag is repeatable.

```sh
composed add redis --chart bitnami/redis \
  --repo https://charts.bitnami.com/bitnami \
  --set architecture=standalone \
  --set auth.enabled=false
```

This produces:

```yaml
services:
  redis:
    x-helm:
      chart: bitnami/redis
      repo: https://charts.bitnami.com/bitnami
      values:
        architecture: standalone
        auth:
          enabled: "false"
```

Nested keys use dot notation (`auth.enabled`) and are expanded into nested maps in the YAML output.

**When to use:** Quick overrides for a small number of values -- image tags, replica counts, feature toggles. Keeps everything in one file with no external dependencies.

---

## Merge file with --values

Use `--values` to read a YAML values file and merge its contents inline into `composed.yaml` at the time you run `composed add`. The file is read once; after that, the values live inside `composed.yaml` and the original file is no longer needed.

```sh
composed add redis --chart bitnami/redis \
  --repo https://charts.bitnami.com/bitnami \
  --values ./my-redis-values.yaml
```

If `my-redis-values.yaml` contains:

```yaml
architecture: standalone
auth:
  enabled: false
  password: "redis-secret"
master:
  persistence:
    size: 2Gi
```

The result in `composed.yaml` is:

```yaml
services:
  redis:
    x-helm:
      chart: bitnami/redis
      repo: https://charts.bitnami.com/bitnami
      values:
        architecture: standalone
        auth:
          enabled: false
          password: "redis-secret"
        master:
          persistence:
            size: 2Gi
```

If `--set` flags are also provided, they are applied on top of the merged file contents.

**When to use:** Importing a values file that you want baked into `composed.yaml`. Useful when you have an existing values file and want a self-contained config with no external file references.

---

## Reference file with --values-file

Use `--values-file` to store a path reference in `composed.yaml`. The file is not read at add time -- it is loaded at build time when `composed build` runs.

```sh
composed add redis --chart bitnami/redis \
  --repo https://charts.bitnami.com/bitnami \
  --values-file ./redis-values.yaml
```

This produces:

```yaml
services:
  redis:
    x-helm:
      chart: bitnami/redis
      repo: https://charts.bitnami.com/bitnami
      values_file: ./redis-values.yaml
```

The path is resolved relative to the location of `composed.yaml` at build time. The file must exist when `composed build` runs.

**When to use:** Keeping values in a separate file that is version-controlled independently, shared across multiple services or environments, or too large to inline comfortably. This also makes it straightforward to diff changes to chart values over time.

---

## Combining methods

All three methods can be used together. The merge priority applies:

```sh
composed add redis --chart bitnami/redis \
  --repo https://charts.bitnami.com/bitnami \
  --values-file ./redis-values.yaml \
  --set architecture=standalone \
  --set image.tag=7.2
```

Result in `composed.yaml`:

```yaml
services:
  redis:
    x-helm:
      chart: bitnami/redis
      repo: https://charts.bitnami.com/bitnami
      values:
        architecture: standalone
        image:
          tag: "7.2"
      values_file: ./redis-values.yaml
```

At build time:

1. `redis-values.yaml` is loaded as the base values.
2. Inline `values:` (`architecture`, `image.tag`) are merged on top.
3. The combined values are passed to `helm template`.

---

## Scaffolding values files with --helm-values

The `init --helm-values` command generates default values files for every Helm service in your `composed.yaml` that does not already have a `values_file` reference.

```sh
composed init --helm-values
```

For each qualifying service, Composed runs `helm show values <chart>` to fetch the chart's default values and writes them to a file named `<service-name>.values.yaml` in the same directory as `composed.yaml`. It then adds the `values_file` reference automatically.

**Before:**

```yaml
services:
  redis:
    x-helm:
      chart: bitnami/redis
      repo: https://charts.bitnami.com/bitnami
      version: "18.x"
      values:
        architecture: standalone
```

**After running `composed init --helm-values`:**

```yaml
services:
  redis:
    x-helm:
      chart: bitnami/redis
      repo: https://charts.bitnami.com/bitnami
      version: "18.x"
      values:
        architecture: standalone
      values_file: redis.values.yaml
```

A new file `redis.values.yaml` is created containing the full default values from the chart. You can then edit this file to customize the deployment, with your inline `values:` overrides still taking priority at build time.

Services that already have a `values_file` set are skipped. Existing files on disk are not overwritten.
