# Review Guidelines

Instructions for AI reviewers (CodeRabbit, Devin Review, Qodo, etc.) and human reviewers.

## Ground Rules

1. **DESIGN.md is the spec.** Code that contradicts DESIGN.md is a bug. But DESIGN.md itself may need updating — flag the contradiction, don't assume which side is wrong.
2. **Tests must compile and pass.** All tests run in CI (`go test ./...`). If you think a function is "undefined", search the entire `cmd/` package — it may live in a different file within the same package.
3. **`examples/` is demo code.** Don't file bugs against example charts, templates, or values files. They exist to show usage patterns, not to be production-ready Helm charts.

## Known Decisions (not bugs)

These are intentional design choices that reviewers repeatedly flag. Do NOT re-report them.

### TCP probe uses `/dev/tcp` (bash-ism)

The TCP healthcheck translation uses `cat < /dev/tcp/localhost/PORT`. This is a bash-specific feature and is specified in DESIGN.md. It is a known limitation documented in the spec. Alternatives (netcat, etc.) have worse availability in minimal images.

### ClusterIP services get port mappings

DESIGN.md says ClusterIP services have "no port mapping". The code intentionally maps ports anyway because Docker Compose requires explicit port exposure for inter-service communication (unlike K8s where ClusterIP provides implicit routing). The test acknowledges this: `"ClusterIP → still maps ports for Compose"`. This is a pragmatic deviation — flag it only if it causes actual breakage.

### Topological sort has no cycle detection

`topoSort` in `cmd/build.go` does not detect dependency cycles. Docker Compose itself detects cycles at startup and gives a clear error. Adding cycle detection here is low priority — it would only improve the error message timing, not correctness.

### `resolveString` uses `strings.Replace` (single pass)

Placeholder resolution does one pass per variable. Recursive/chained placeholders (`${a}` expanding to `${b}`) are not supported by design. This is not a bug.

## Common False Positives

### "Function X is undefined" in test files

The `cmd/` package has multiple `.go` files (`build.go`, `config.go`, `resolve.go`, `oci.go`). Functions like `parseSetValues`, `parseEnvValues`, and `deriveComponentName` live in `config.go` but are tested from `build_test.go`. Go compiles the whole package — cross-file references within the same package are valid. **Always search all files in the package before reporting undefined functions.**

### `rangeValCopy` warnings

Flagging `for _, x := range slice` as copying large structs is a micro-optimization. In this codebase, the structs are K8s API types iterated once during translation. The copy overhead is negligible compared to YAML parsing and HTTP calls. Don't flag these unless the loop is in a hot path (none are).

### "Shell injection" in healthcheck translation

`translateProbe` interpolates HTTP paths into shell commands. The input is a K8s manifest — authored by the same team deploying it. This is trusted input, not user-supplied web input. Standard shell quoting is sufficient; this is not a security vulnerability.

### "Secret data double-decoded"

K8s `corev1.Secret.Data` contains raw bytes (already base64-decoded by the API). The code uses `.Data` directly as string values — it does NOT base64-decode again. If you see `base64Decode`, verify it's actually called on `.Data` before reporting.

## What We DO Want Flagged

### Real bugs

- Nil pointer dereferences (especially on map writes without nil checks)
- Contradictions between code and DESIGN.md (flag both sides)
- Missing error handling on I/O operations
- Panic-inducing edge cases (empty slices, missing map keys)

### Spec compliance

- Translation rules that don't match DESIGN.md tables
- CLI flag behaviour that contradicts documented usage
- Output format changes (YAML key ordering, field names)

### Test gaps

- Exported functions without any test coverage
- Translation rules from DESIGN.md without corresponding test cases
- Bug fixes without regression tests

### Determinism

- Map iteration without sorting (Go maps are unordered)
- Non-deterministic output in generated YAML or header comments
- Timestamp or random-dependent behaviour

## Severity Calibration

| Severity | Use when | Examples |
|----------|----------|---------|
| Critical | Code won't compile or panics on valid input | nil deref, missing import, type mismatch |
| Major | Wrong behaviour on common paths | Wrong translation rule, dropped config field, spec contradiction |
| Minor | Edge cases, style, optimisations | Cycle detection, rangeValCopy, comment accuracy |
| Info | Suggestions, not issues | Alternative approaches, documentation improvements |

**Don't inflate severity.** A style suggestion is not "Critical". A missing cycle detection in a fallback path is not "Major". Over-reporting erodes reviewer trust.
