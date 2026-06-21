# nats-runner Agent Guidelines

A Go CLI tool for sending NATS messages (Request-Reply, Publish, JetStream) driven by TOML templates and a 5-level variable resolution engine (Go `text/template`). Ships with an interactive two-pane TUI for building requests with a live JSON preview.

See [design.md](design.md) (Traditional Chinese) for full design rationale, [metering.md](metering.md) for metering API context, and [report.md](report.md) for JetStream reporting patterns.

---

## Build & Test

```bash
# Build (static binary)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=1.0.0" -o nats-runner .

# Run all tests
go test ./...
```

No Makefile exists; use the commands above directly.

---

## Architecture

```
internal/
  cli/        — stdlib `flag`-based CLI: root command + config subcommand
  config/     — TOML config loader (connections, global config, funcs)
  domain/     — Core types only; zero external dependencies
  nats/       — NATS connection + unified Exec (req / pub / js), returns strings
  template/   — TOML template loader and entry lookup
  vars/       — 5-level variable resolver over Go text/template
  logger/     — per-run structured log writer (logs/*.log)
  tui/        — interactive two-pane TUI (bubbletea): form + live JSON preview
templates/    — TOML template files (one file per domain)
configs/      — connection files (configs/<name>.toml)
funcs/        — shell-function definitions (funcs/<name>.toml)
values/       — optional values files (.toml / .json) for --values
scripts/      — Python helpers for multi-param builds
```

CLI entry: `nats-runner.go` → `internal/cli/root.go` → `Execute()`.
The TUI is opt-in: launch it with the `tui` subcommand or `-i` flag. Bare
`nats-runner` prints usage and exits (never auto-launches the TUI), keeping the
tool safe for scripts and non-interactive use.

---

## Key Conventions

### Template entries (TOML)
```toml
[entry_name]
subject  = "nats.subject"
mode     = "req"           # "req" | "pub" | "js"
defaults = { key = "val" }
body     = '{"field": "{{ .var }}", "free_text": {{ .desc | toJson }}}'

[entry_name.stream]        # JetStream only — optional auto-create
create   = true
name     = "STREAM_NAME"
subjects = ["pattern.*"]
storage  = "file"          # "file" | "memory"
```

Bodies are rendered with Go `text/template`:
- Data variables: `{{ .var }}` (note the leading dot).
- Built-in functions: `{{ uuid }}`, `{{ now }}`, `{{ now_ms }}`, `{{ now_iso }}` (no dot).
- Pipes: `{{ .arr | toJson }}`, `{{ .s | trim }}`.
- **JSON safety:** wrap free-text string fields in `{{ .field | toJson }}` (adds quotes + escapes). The renderer also validates that JSON-looking output is valid and errors clearly otherwise.

### Variable resolution order (highest → lowest)
1. CLI `key=value` params
2. `--values` files (`.toml` / `.json`, repeatable; later files win)
3. `defaults` in the template entry
4. Shell functions in `funcs/<name>.toml` — executed only when referenced, once per run
5. Built-ins: `{{ uuid }}`, `{{ now }}`, `{{ now_ms }}`, `{{ now_iso }}`

### Authentication modes (`auth_mode`)
`creds` | `token` | `nkey` | `none`

---

## Pitfalls

- **Unresolved variable** → stderr error + exit 1 (`missingkey=error`); check spelling and the leading dot (`{{ .var }}`).
- **JetStream stream conflicts** fail loudly by design; do not modify stream config silently.
- **Shell functions** (`funcs/*.toml`) are executed literally via `sh -c` — validate for security before use. They run only when referenced by the body, once per run.
- **`{{ uuid }}` built-in is fresh on every occurrence** — two `{{ uuid }}` in one body produce *different* values. To reuse one value, define a shell function (e.g. `funcs/uuid.toml`) and reference it as `{{ .uuid }}` (function results are stored once per run, so all `{{ .uuid }}` share the value).
- **nkey mode**: only the seed file (`.nk`) is required; the public key is derived automatically.
- **Version** is injected at build time via `-ldflags`; binary shows `dev` if built without it.
- **Build/test** with `GOWORK=off` if the repo sits inside a parent `go.work` that does not list it.

---

## Git Workflow

**Repository:** `https://github.com/wz-cch/nats-runner`

### Branch model
| Branch | Purpose | Direct push |
|--------|---------|-------------|
| `main` | Stable releases only | ✗ — PR only |
| `develop` | Integration branch | ✗ — PR only |
| `feat/<short-name>` | New features | ✓ |
| `fix/<short-name>` | Bug fixes | ✓ |
| `chore/<short-name>` | Maintenance (deps, CI, docs) | ✓ |

### Rules
- **Never commit directly to `main` or `develop`** — always open a PR.
- Branch from `develop`; target `develop` on PR; merge `develop` → `main` for releases.
- Commit messages **must be in English**, follow [Conventional Commits](https://www.conventionalcommits.org/):
  `feat(scope): add X`, `fix(scope): resolve Y`, `chore: update Z`
- Subject line ≤ 72 characters, imperative mood.
- Do **not** commit compiled binaries (`main`, `nats-runner`, `nats-runner-*`), credentials (`*.creds`, `*.nk`), or local configs.

### Workflow (agent steps)
1. `git checkout develop && git pull origin develop`
2. `git checkout -b feat/<name>`
3. Make changes → `go test ./...` passes
4. `git add <only relevant files>` — never `git add .` blindly
5. Commit with English Conventional Commit message
6. `git push origin feat/<name>`
7. Open PR targeting `develop` on GitHub
