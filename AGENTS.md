# Project Rules — composed

## Spec-Driven Development (SDD)

**DESIGN.md is the single source of truth for all behaviour.**

Before implementing any feature or change:

1. Read `DESIGN.md` to understand the existing spec.
2. If the change alters user-facing behaviour, CLI interface, translation rules, or config format: **update DESIGN.md first**, then implement.
3. Code that contradicts DESIGN.md is a bug. DESIGN.md that contradicts desired behaviour needs a spec update before code changes.
4. PR reviews must verify DESIGN.md consistency — reject code changes that add undocumented behaviour.

## Test-Driven Development (TDD)

**All code must have tests. No exceptions.**

1. **Write the failing test first**, then implement the code to make it pass.
2. Every exported function must have at least one test case.
3. Translation rules (K8s -> Compose) must have table-driven tests covering each row in DESIGN.md.
4. Bug fixes must include a regression test that reproduces the bug before the fix.
5. Target **>80% line coverage**. Run `go test -coverprofile=coverage.out ./...` and check.

### Test organisation

| Package | Test file | What to test |
|---------|-----------|-------------|
| `internal/config` | `config_test.go` | Parse, Load, ResolveRefs, ServiceType |
| `internal/k8s` | `parser_test.go` | Multi-doc split, typed parsing, skipped resources |
| `internal/translate` | `translate_test.go` | Every translation rule from DESIGN.md |
| `internal/compose` | `emit_test.go` | Round-trip: model -> YAML -> verify structure |
| `internal/merge` | `merge_test.go` | Union merge, conflict resolution, dedup |
| `cmd` | `build_test.go`, `config_test.go`, `oci_test.go` | CLI helpers, topoSort, flattenValues, parseOCIRef, classifyManifest |

### Running tests

```bash
go test ./...                                    # all tests
go test -v ./internal/translate/...              # verbose, one package
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out  # coverage
```

## Build & verify

```bash
go build -o composed .       # binary
go vet ./...                 # static analysis
go test ./...                # tests
```

## Go module

Module path: `github.com/docker-x/composed`

Use `go get -u <pkg>` to update dependencies. Keep `go.mod` tidy with `go mod tidy`.

## Code conventions

- No external frameworks beyond stdlib + cobra + yaml.v3 + k8s.io types.
- Internal packages under `internal/` — not importable by external code.
- Deterministic output: sort maps by key before emitting YAML.
- All translation follows the rules in DESIGN.md `Translation Rules` section.
