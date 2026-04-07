---
sidebar_position: 1
---

# Installation

This page covers how to install `composed`.

## Option 1: Homebrew (macOS / Linux)

```bash
brew install docker-x/tap/composed
```

## Option 2: Download a Binary

Pre-built binaries are available for Linux, macOS, and Windows on the
[GitHub Releases](https://github.com/docker-x/composed/releases) page.

1. Download the archive for your OS and architecture.
2. Extract it and move the binary to a directory on your PATH:

```bash
# Example for Linux amd64
tar xzf composed_*_linux_amd64.tar.gz
sudo mv composed /usr/local/bin/
```

## Option 3: Go Install

If you have Go 1.26+ installed:

```bash
go install github.com/docker-x/composed@latest
```

Make sure `$GOBIN` (usually `$HOME/go/bin`) is on your PATH.

## Option 4: Build from Source

```bash
git clone https://github.com/docker-x/composed.git
cd composed
go build -o composed .
sudo mv composed /usr/local/bin/
```

## Runtime Dependencies

| Dependency | Minimum Version | Purpose |
|------------|-----------------|---------|
| [Docker](https://docs.docker.com/get-docker/) | 20.10+ | Runs the resulting compose stack |
| [Helm](https://helm.sh/docs/intro/install/) | 3.x | Only needed if your stack includes `x-helm` charts |

Docker must be running before you use `composed up` or `composed down`. Helm is
only needed at build time to fetch and render charts -- if your stack contains
only plain images and compose-file includes, you can skip it.

## Verify the Installation

```bash
composed --version
```

Expected output (version and commit will vary):

```
composed version 0.3.1 (commit: abc1234, built: 2025-01-15T10:00:00Z)
```

## Next Steps

With `composed` installed, head to the [Quick Start](quick-start.md) guide to
create your first stack.
