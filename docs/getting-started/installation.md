# Installation

This page covers how to install `composed` and its prerequisites.

## Prerequisites

| Dependency | Minimum Version | Purpose |
|------------|-----------------|---------|
| [Go](https://go.dev/dl/) | 1.22+ | Required to install or build `composed` |
| [Docker](https://docs.docker.com/get-docker/) | 20.10+ | Runs the resulting compose stack |
| [Helm](https://helm.sh/docs/intro/install/) | 3.x | Required only if your stack includes `x-helm` charts |

Docker must be running before you use `composed up` or `composed down`. Helm is
only needed at build time to fetch and render charts -- if your stack contains
only plain images and compose-file includes, you can skip it.

## Option 1: Go Install

If you have Go 1.22 or later on your PATH, the fastest way to get `composed` is:

```bash
go install github.com/docker-x/composed@latest
```

This downloads the source, compiles it, and places the binary in `$GOBIN`
(usually `$HOME/go/bin`). Make sure that directory is on your PATH:

```bash
export PATH="$HOME/go/bin:$PATH"
```

To install a specific version instead of `latest`:

```bash
go install github.com/docker-x/composed@v0.3.1
```

## Option 2: Build from Source

Clone the repository and build with `make` or `go build`:

```bash
git clone https://github.com/docker-x/composed.git
cd composed
go build -o composed .
```

This produces a `composed` binary in the current directory. Move it somewhere on
your PATH:

```bash
sudo mv composed /usr/local/bin/
```

Alternatively, use the Makefile target which does the same thing:

```bash
make build
```

### Running the test suite

After cloning, you can verify everything works:

```bash
make test
```

## Verify the Installation

Confirm that the binary is reachable and prints its version:

```bash
composed --version
```

Expected output (version and commit will vary):

```
composed version 0.3.1 (commit: abc1234, built: 2025-01-15T10:00:00Z)
```

If the command is not found, check that the directory containing the binary is
on your PATH.

## Next Steps

With `composed` installed, head to the [Quick Start](quick-start.md) guide to
create your first stack.
