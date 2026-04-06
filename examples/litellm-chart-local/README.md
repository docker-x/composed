# LiteLLM Helm Chart (Local Wrapper)

A local wrapper chart that declares LiteLLM as a subchart dependency.
The OCI chart is pinned in `Chart.yaml` and pulled by `helm dep update`
at build time. No need to download the chart manually.

## How these files were created

```bash
composed init
# create Chart.yaml with dependencies: section pointing to the OCI chart
# create litellm-helm.values.yaml with just your overrides
composed add . --values-file litellm-helm.values.yaml
composed up
```

`composed build` automatically runs `helm dependency update` for local charts,
pulling the subchart into `charts/`. Then it renders templates and translates
to Compose.

## Chart.yaml

```yaml
dependencies:
  - name: litellm-helm
    version: "1.82.3"
    repository: "oci://docker.litellm.ai/berriai"
```

Subchart values are overridden under the dependency name in the values file:

```yaml
litellm-helm:
  image:
    tag: main-stable
  masterkey: sk-1234
```

To see all available options:

```bash
helm show values oci://docker.litellm.ai/berriai/litellm-helm
```

## Files

| File | Tracked | Source |
|------|---------|--------|
| `Chart.yaml` | yes | Hand-written (declares subchart dependency) |
| `composed.yaml` | yes | `composed init` + `composed add` |
| `litellm-helm.values.yaml` | yes | Hand-written overrides |
| `charts/` | no | `helm dep update` (gitignored) |
| `Chart.lock` | no | `helm dep update` (gitignored) |
| `docker-compose.yaml` | no | `composed build` (gitignored) |
