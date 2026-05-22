# nats-runner Agent Guidelines

A zero-dependency Go CLI tool for sending NATS messages (Request-Reply, Publish, JetStream) driven by TOML templates and a 4-level variable resolution engine.

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
  cli/        — Cobra CLI: root command + config subcommand
  config/     — TOML config loader (AppConfig, ConnectionConfig)
  domain/     — Core types only; zero external dependencies
  nats/       — NATS connection + ExecReq / ExecPub / ExecJS
  template/   — TOML template loader and entry lookup
  vars/       — 4-level variable resolver (CLI > defaults > functions > builtins)
templates/    — TOML template files (one file per domain)
configs/      — Example config files
scripts/      — Python helpers for multi-param builds
```

CLI entry: `nats-runner.go` → `internal/cli/root.go` → `Execute()`

---

## Key Conventions

### Template entries (TOML)
```toml
[entry_name]
subject  = "nats.subject"
mode     = "req"           # "req" | "pub" | "js"
defaults = { key = "val" }
body     = '{"field": "{{var}}"}'

[entry_name.stream]        # JetStream only — optional auto-create
create   = true
name     = "STREAM_NAME"
subjects = ["pattern.*"]
storage  = "file"          # "file" | "memory"
```

### Variable resolution order (highest → lowest)
1. CLI `key=value` params
2. `defaults` in template entry
3. Shell `[functions]` in config.toml (results cached per variable name per run)
4. Built-ins: `{{now}}`, `{{now_ms}}`, `{{now_iso}}`, `{{uuid}}`

### Authentication modes (`auth_mode`)
`creds` | `token` | `nkey` | `none`

---

## Pitfalls

- **Unresolved variable** → stderr error + exit 1; check spelling matches exactly.
- **JetStream stream conflicts** fail loudly by design; do not modify stream config silently.
- **Shell functions** in config are executed literally — validate for security before use.
- **`{{uuid}}` appears twice** in one body → same value (cached). Use `{{uuid2}}` etc. for distinct values.
- **nkey mode**: only the seed file (`.nk`) is required; the public key is derived automatically.
- **Version** is injected at build time via `-ldflags`; binary shows `dev` if built without it.

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
