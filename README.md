# nats-runner

A Go CLI tool for sending NATS messages driven by TOML templates and a 5-level
variable resolution engine (Go `text/template`). It ships with an interactive
two-pane TUI that builds a request and shows a live JSON preview before sending.

也提供[正體中文說明](README.zh-TW.md)。

---

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Interactive Mode (TUI)](#interactive-mode-tui)
- [Templates](#templates)
- [Variable Resolution](#variable-resolution)
- [Authentication](#authentication)
- [Examples](#examples)
- [Project Structure](#project-structure)

---

## Features

- Three NATS delivery modes: **Request-Reply** (`req`), **Publish** (`pub`), **JetStream** (`js`)
- TOML-driven templates — no recompilation needed for new API calls
- 5-level variable resolution: **CLI args → `--values` files → template defaults → shell functions → built-ins**
- Go `text/template` bodies: `{{ .var }}` data variables, `{{ uuid }}` built-ins, `{{ .x | toJson }}` pipes
- JSON-safe rendering: free-text via `{{ .field | toJson }}`, plus validation of JSON-looking output
- Built-in functions: `{{ uuid }}`, `{{ now }}`, `{{ now_ms }}`, `{{ now_iso }}`
- Shell functions (`funcs/*.toml`) executed only when referenced, once per run
- Interactive two-pane TUI with live preview, loop runner, and per-run logging
- Authentication: `creds`, `token`, `nkey`, `none`; TLS incl. mTLS
- JetStream stream auto-creation
- Single static binary — no runtime dependencies

---

## Installation

Requires Go 1.21+ (module targets a newer toolchain; see `go.mod`).

```bash
git clone https://github.com/wz-cch/nats-runner.git
cd nats-runner

# Linux / AMD64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -ldflags "-X main.version=1.0.0" -o nats-runner .

# Linux / ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -ldflags "-X main.version=1.0.0" -o nats-runner-arm64 .

./nats-runner --version
```

> If the repo lives inside a parent Go workspace that does not list it, prefix
> build/test commands with `GOWORK=off`.

---

## Configuration

### Connections — `configs/<name>.toml`

Each NATS environment is one file under `configs/`, referenced by **name** with `-c`:

```toml
# configs/metering.toml
[connection]
url            = "nats://nats-cluster.example.com:4222"
auth_mode      = "creds"   # creds | token | nkey | none
creds_file     = "./auth/user.creds"
token          = ""
nkey_seed_file = ""
timeout_ms     = 5000

[connection.tls]            # optional
ca_cert              = ""
client_cert          = ""
client_key           = ""
insecure_skip_verify = false
```

`-c metering` resolves to `configs/metering.toml`. A path containing `/` is used as-is.

### Global config — `~/.nats-runner.toml`

Tool-wide defaults (written by `config set`):

```toml
default_connection = "metering"
template_dir       = "templates"
funcs_dir          = "funcs"
values_dir         = "values"
```

### Shell functions — `funcs/<name>.toml`

Each dynamic variable backed by a shell command is its own file:

```toml
# funcs/uuid.toml
command = "uuidgen"
desc    = "Generate a random UUID v4"
```

Referenced in a body as `{{ .uuid }}`. Functions are loaded from `funcs_dir`
and executed only when referenced, once per run.

### Config subcommands

```bash
./nats-runner config set metering          # set default connection (validates configs/metering.toml)
./nats-runner config set --funcs-dir funcs --values-dir values --template-dir templates
./nats-runner config show                  # show effective global config + connection
./nats-runner config list                  # list connections under configs/
```

---

## Usage

```
nats-runner [flags] -t <template> -n <entry> [key=value ...]
```

| Flag | Description |
|------|-------------|
| `-t` | Path to the TOML template file |
| `-n` | Template entry name |
| `-c` | Connection name (e.g. `metering`) or path; falls back to `default_connection` |
| `--values` | Values file (`.toml`/`.json`), repeatable; later files take priority |
| `--loop` | Number of executions (`0` = infinite) |
| `--interval` | Delay between loop iterations (e.g. `60s`, `500ms`) |
| `-i` | Launch the interactive TUI (same as the `tui` subcommand) |
| `--version` | Show version |

Extra `key=value` arguments are the highest-priority variables and override everything else.
The TUI is **opt-in** (`nats-runner tui` or `-i`) — running `nats-runner` with no
arguments prints usage and exits, so the tool stays script/automation friendly.

---

## Interactive Mode (TUI)

Launch it explicitly with `nats-runner tui` (or `nats-runner -i`) to open the two-pane editor:

- **Left pane** — focusable fields: connection, template file, entry, then the
  variables the entry's body actually needs (discovered by scanning the body),
  followed by loop options and values files.
- **Right pane** — a live preview of the rendered JSON payload (shell functions
  are stubbed as `<func:NAME>`; built-ins render live), plus the subject and mode.
- **Bottom** — the equivalent CLI command for the current selection.

Keys: `↑/↓` move between rows · `←/→` switch connection/template/entry inline
(no picker) · `Enter` open the full list / toggle · type directly on variable rows
· `Ctrl+R` run · `Ctrl+C` stop/quit. Choosing a template auto-selects its first
entry so the variable rows appear immediately. Each run writes a log to `logs/`.

---

## Templates

One TOML file per domain, one section per operation. See [`templates/`](templates/) for real examples.

```toml
[entry_name]
subject  = "nats.subject.path"
mode     = "req"               # req | pub | js
defaults = { role = "member" }
body     = '''
{
  "id":   "{{ uuid }}",
  "role": "{{ .role }}",
  "note": {{ .note | toJson }}
}
'''

# JetStream only — optional stream auto-creation
[entry_name.stream]
create   = true
name     = "STREAM_NAME"
subjects = ["pattern.*"]
storage  = "file"              # file | memory
```

| Mode | Behaviour |
|------|-----------|
| `req` | Request-Reply — waits for the reply, prints pretty JSON |
| `pub` | Publish — fire-and-forget, prints `Published to: <subject>` |
| `js`  | JetStream publish — prints `JetStream published to: <subject> (seq: N)` |

**JSON safety:** wrap free-text string fields in `{{ .field | toJson }}` (it adds
quotes and escapes `"`, `\`, newlines). The renderer also validates JSON-looking
output and fails with a clear error if it is malformed.

---

## Variable Resolution

Bodies render through Go `text/template`. Data variables use a leading dot
(`{{ .name }}`); built-in functions do not (`{{ uuid }}`). Values are resolved
in this priority order (highest → lowest):

```
1. CLI key=value params          ← highest
2. --values files (.toml/.json)
3. defaults in the template entry
4. shell functions (funcs/*.toml, referenced as {{ .name }})
5. built-in functions            ← lowest
```

| Built-in | Value | In JSON |
|----------|-------|---------|
| `{{ uuid }}`    | Random UUID v4 (fresh on every occurrence) | string — quote it: `"{{ uuid }}"` |
| `{{ now }}`     | Unix timestamp (seconds, UTC) | number — leave unquoted |
| `{{ now_ms }}`  | Unix timestamp (milliseconds, UTC) | number — leave unquoted |
| `{{ now_iso }}` | ISO 8601 / RFC3339 (UTC) | string — quote it: `"{{ now_iso }}"` |

> `{{ uuid }}` and `{{ now_iso }}` emit strings and **must be quoted** in a JSON
> body; an unquoted `{{ now_iso }}` produces invalid JSON.

> The built-in `{{ uuid }}` produces a **different** value at each occurrence.
> `{{ now }}`/`{{ now_ms }}`/`{{ now_iso }}` are captured once per run and stay
> consistent. To reuse a single generated value across a body, define a shell
> function (e.g. `funcs/uuid.toml`) and reference it as `{{ .uuid }}` — function
> results are stored once per run.

An unresolved variable exits with code 1 and an error on stderr (`missingkey=error`).

---

## Authentication

| `auth_mode` | Required field |
|-------------|----------------|
| `creds` | `creds_file` |
| `token` | `token` |
| `nkey`  | `nkey_seed_file` (`.nk` seed only — public key is auto-derived) |
| `none`  | — |

---

## Examples

### Request-Reply

```bash
./nats-runner -c metering -t templates/srp_types.toml -n create \
  srp_type=ai-service-package \
  description="AI Service Package" \
  resource1=api resource2=compute \
  metric_field=aitoken
```

### Publish

```bash
./nats-runner -c metering -t templates/usage_report.toml -n submit \
  tenant_id=tenant-001 resource=compute amount=500
```

### JetStream

```bash
./nats-runner -c metering -t templates/usage_stats.toml -n record \
  tenant_id=tenant-001 stat_key=api_calls value=1200
```

### Loop / load test

```bash
./nats-runner -c metering -t templates/usage_stats.toml -n record \
  tenant_id=tenant-001 --loop 100 --interval 200ms
```

### Values file

```bash
./nats-runner -c metering -t templates/srp_types.toml -n create_multi_metrics \
  --values values/base.toml --values values/override.json
```

---

## Project Structure

```
nats-runner.go     — Entry point
internal/
  cli/             — stdlib flag CLI (root, config subcommand)
  config/          — TOML loaders (connections, global config, funcs)
  domain/          — Core types (zero dependencies)
  nats/            — NATS connection + unified Exec (req/pub/js)
  template/        — Template loader & entry lookup
  vars/            — 5-level variable resolver over text/template
  logger/          — per-run structured log writer
  tui/             — interactive two-pane TUI (bubbletea)
templates/         — TOML template files (one per domain)
configs/           — connection files (configs/<name>.toml)
funcs/             — shell-function definitions (funcs/<name>.toml)
values/            — optional values files for --values
scripts/           — Python helper scripts
```

---

## Development

```bash
go test ./...        # add GOWORK=off if inside a parent go.work
go vet ./...
```

Contributions follow a branch-based workflow (`feat/`, `fix/`, `chore/` → PR to
`develop`). Commit messages must be in **English** and follow
[Conventional Commits](https://www.conventionalcommits.org/). See [AGENTS.md](AGENTS.md).

---

## License

MIT
