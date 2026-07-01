# Repository Guidelines

## Project Structure & Module Organization

This is a Go module: `github.com/IrineSistiana/mosdns/v5`.

- `main.go` wires the CLI and imports enabled plugins.
- `coremain/` contains config loading, service startup, and plugin lifecycle.
- `pkg/` holds reusable libraries such as DNS utilities, server handlers, upstream transports, cache, matchers, and zone-file parsing.
- `plugin/` contains configurable runtime extensions, grouped by role: `data_provider/`, `matcher/`, `executable/`, `server/`, and `mark/`.
- `tools/` adds CLI helpers such as config generation/conversion.
- `scripts/` contains packaging and platform helper scripts.
- Tests live beside implementation files as `*_test.go`.

Avoid committing large local data files such as `geoip.dat`, `geosite.dat`, generated certs, or local-only config experiments unless explicitly requested.

## Build, Test, and Development Commands

- `go test ./...` runs the full test suite.
- `go test ./plugin/...` runs plugin-focused tests.
- `go test ./pkg/upstream/...` is useful after transport or TFO changes.
- `go run . start -c config.yaml -d "$(pwd)"` starts mosdns with a local config.
- `go run . config gen config.yaml` generates a template config.
- `go build ./...` verifies all packages compile.

Use `gofmt -w <files>` before committing Go changes.

## Coding Style & Naming Conventions

Follow standard Go formatting and idioms. Package names are lower-case and directory-oriented. Plugin packages should expose a clear `PluginType` constant and register themselves in `init()`. If a plugin must be available in normal builds, add its blank import in `plugin/enabled_plugins.go`.

Keep configuration keys stable and YAML tags explicit. Prefer small, focused helpers over broad refactors.

## Testing Guidelines

Use Go’s built-in `testing` package; existing tests may also use `testify`. Put tests next to the code under test and name them `TestXxx`. For plugins, cover both provider loading and matcher/executable behavior. For network-facing changes, include local loopback tests when possible and document any required kernel/sysctl assumptions.

## Commit & Pull Request Guidelines

Recent commits use short imperative messages, for example `add tcp fast open support` or `enable geosite matcher plugin`. Keep commits scoped to one logical change.

Pull requests should include a short problem statement, the implementation summary, tests run, and any config or compatibility notes. Link issues when applicable, and include logs or command output for networking behavior changes.

## Security & Configuration Tips

Do not commit secrets, private upstream URLs, generated private keys, or local DNS datasets. Keep sample configs deterministic and prefer loopback addresses for tests and examples.
