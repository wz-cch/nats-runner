# nats-runner

A zero-dependency CLI tool for sending NATS messages driven by TOML templates and a 4-level variable resolution engine.

也提供[正體中文說明](README.zh-TW.md)。

---

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Templates](#templates)
- [Variable Resolution](#variable-resolution)
- [Authentication](#authentication)
- [Examples](#examples)
- [Project Structure](#project-structure)

---

## Features

- Three NATS delivery modes: **Request-Reply**, **Publish**, **JetStream**
- TOML-driven templates — no recompilation needed for new API calls
- 4-level variable resolution: CLI args → template defaults → shell functions → built-ins
- Built-in variables: `{{uuid}}`, `{{now}}`, `{{now_ms}}`, `{{now_iso}}`
- Shell function integration with per-run result caching
- Authentication: `creds`, `token`, `nkey`, `none`
- TLS support: CA cert, mTLS, insecure skip
- JetStream stream auto-creation
- Single static binary — no runtime dependencies

---

## Installation

Requires Go 1.21+

```bash
git clone https://github.com/wz-cch/nats-runner.git
cd nats-runner

# Linux / AMD64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -ldflags "-X main.version=1.0.0" -o nats-runner .

# Linux / ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -ldflags "-X main.version=1.0.0" -o nats-runner-arm64 .
```

```bash
./nats-runner --version
```

---

## Configuration

Create a config file at `~/.nats-runner.toml` (or specify with `-c`):

```toml
[connection]
url            = "nats://nats-cluster.example.com:4222"
auth_mode      = "creds"   # creds | token | nkey | none
creds_file     = "./auth/user.creds"
token          = ""
nkey_seed_file = ""
timeout_ms     = 5000

[connection.tls]
ca_cert              = ""
client_cert          = ""
client_key           = ""
insecure_skip_verify = false

[functions]
uuid = "uuidgen"   # shell command — result is cached per variable name per run
```

See [`configs/metering.toml`](configs/metering.toml) for a full example.

### Manage config path

```bash
./nats-runner config set /path/to/config.toml   # store as global default
./nats-runner config show                        # display current default
```

---

## Usage

```
nats-runner [flags] -t <template> -n <entry> [key=value ...]
```

| Flag | Description | Required |
|------|-------------|----------|
| `-t` | Path to TOML template file | ✓ |
| `-n` | Template entry name | ✓ |
| `-c` | Config file path (overrides default) | — |
| `--version` | Show version | — |

Extra `key=value` arguments are passed as variables and override template defaults.

---

## Templates

One TOML file per domain, one section per operation. See [`templates/`](templates/) for real-world examples.

```toml
[entry_name]
subject  = "nats.subject.path"
mode     = "req"               # req | pub | js
defaults = { role = "member" }
body     = '''
{
  "id":   "{{uuid}}",
  "role": "{{role}}"
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
| `req` | Request-Reply — waits for response, prints pretty JSON |
| `pub` | Publish — fire-and-forget, prints confirmation |
| `js`  | JetStream publish — prints sequence number |

---

## Variable Resolution

Variables in `{{name}}` placeholders are resolved in this priority order (highest → lowest):

```
1. CLI key=value params
2. defaults in template entry
3. [functions] in config.toml  (shell commands, result cached per run)
4. Built-in variables
```

| Built-in | Value |
|----------|-------|
| `{{uuid}}`    | Random UUID v4 |
| `{{now}}`     | Unix timestamp (seconds) |
| `{{now_ms}}`  | Unix timestamp (milliseconds) |
| `{{now_iso}}` | ISO 8601 / RFC3339 (UTC) |

> `{{uuid}}` appearing multiple times in one body returns the **same value** (cached per run).  
> Use distinct names (`{{uuid2}}`, `{{uuid3}}`) for independent UUIDs.

An unresolved variable exits with code 1 and an error on stderr.

---

## Authentication

| `auth_mode` | Required field |
|-------------|----------------|
| `creds` | `creds_file` |
| `token` | `token` |
| `nkey`  | `nkey_seed_file` (`.nk` seed file only — public key is auto-derived) |
| `none`  | — |

---

## Examples

### Request-Reply

```bash
./nats-runner -t templates/srp_types.toml -n create \
  srp_type=ai-service-package \
  description="AI Service Package" \
  resource1=api resource2=compute \
  metric_field=aitoken
```

### Publish

```bash
./nats-runner -t templates/usage_report.toml -n submit \
  tenant_id=tenant-001 resource=compute amount=500
```

### JetStream

```bash
./nats-runner -t templates/usage_stats.toml -n record \
  tenant_id=tenant-001 stat_key=api_calls value=1200
```

### Python helper for complex payloads

```bash
python scripts/run_with_metrics.py
```

Edit `CONFIG` inside [`scripts/run_with_metrics.py`](scripts/run_with_metrics.py) to customise the payload.

---

## Project Structure

```
nats-runner.go     — Entry point
internal/
  cli/             — CLI commands (root, config)
  config/          — TOML config loader
  domain/          — Core types (zero dependencies)
  nats/            — NATS connection & executors
  template/        — Template loader
  vars/            — Variable resolver
templates/         — TOML template files (one per domain)
configs/           — Example config files
scripts/           — Python helper scripts
```

---

## License

MIT


---

## Features · 功能特色

**English**
- Three NATS delivery modes: **Request-Reply**, **Publish**, **JetStream**
- TOML-driven templates — no recompilation needed for new API calls
- 4-level variable resolution: CLI args → template defaults → shell functions → built-ins
- Built-in variables: `{{uuid}}`, `{{now}}`, `{{now_ms}}`, `{{now_iso}}`
- Shell function integration with per-run caching
- Authentication: `creds`, `token`, `nkey`, `none`
- TLS support: CA cert, mTLS, insecure skip
- JetStream stream auto-creation
- Single static binary — no runtime dependencies

**正體中文**
- 三種 NATS 傳送模式：**Request-Reply**、**Publish**、**JetStream**
- TOML 模板驅動，新增 API 呼叫無需重新編譯
- 四層變數解析：CLI 參數 → 模板預設值 → Shell 函式 → 內建變數
- 內建變數：`{{uuid}}`、`{{now}}`、`{{now_ms}}`、`{{now_iso}}`
- Shell 函式整合，結果於單次執行期間快取
- 認證模式：`creds`、`token`、`nkey`、`none`
- TLS 支援：CA 憑證、mTLS、略過驗證
- JetStream Stream 自動建立
- 單一靜態二進位，無執行期依賴

---

## Installation · 安裝

**Requirements · 需求：** Go 1.21+

```bash
# Clone and build · 下載並編譯
git clone https://github.com/wz-cch/nats-runner.git
cd nats-runner

# Linux / AMD64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -ldflags "-X main.version=1.0.0" -o nats-runner .

# Linux / ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -ldflags "-X main.version=1.0.0" -o nats-runner-arm64 .
```

Verify · 驗證：

```bash
./nats-runner --version
```

---

## Configuration · 設定

Create a config file (default path: `~/.nats-runner.toml`) · 建立設定檔（預設路徑：`~/.nats-runner.toml`）：

```toml
[connection]
url        = "nats://nats-cluster.example.com:4222"
auth_mode  = "creds"          # creds | token | nkey | none
creds_file = "./auth/user.creds"
token      = ""
nkey_seed_file = ""
timeout_ms = 5000

[connection.tls]
ca_cert             = ""
client_cert         = ""
client_key          = ""
insecure_skip_verify = false

[functions]
uuid = "uuidgen"   # shell command — result cached per variable name per run
```

See [`configs/metering.toml`](configs/metering.toml) for a full example · 完整範例請參考 [`configs/metering.toml`](configs/metering.toml)。

### Store config path · 儲存設定檔路徑

```bash
# Set global default · 設定全域預設路徑
./nats-runner config set /path/to/config.toml

# Show current default · 顯示目前預設路徑
./nats-runner config show
```

---

## Usage · 使用方式

```
nats-runner [flags] -t <template> -n <entry> [key=value ...]
```

| Flag | Description · 說明 | Required |
|------|---------------------|----------|
| `-t` | Path to TOML template file · TOML 模板路徑 | ✓ |
| `-n` | Template entry name · 模板條目名稱 | ✓ |
| `-c` | Config file path · 設定檔路徑（覆蓋預設） | — |
| `--version` | Show version · 顯示版本 | — |

Extra `key=value` arguments override template defaults · 額外的 `key=value` 參數可覆蓋模板預設值。

---

## Templates · 模板格式

One TOML file per domain, one section per API operation · 每個業務領域一個 TOML 檔，每個 API 操作一個區段：

```toml
[entry_name]
subject  = "nats.subject.path"    # NATS subject
mode     = "req"                  # req | pub | js
defaults = { role = "member" }    # optional static defaults · 可選靜態預設值
body     = '''
{
  "id":   "{{uuid}}",
  "role": "{{role}}"
}
'''

# JetStream only — optional stream auto-creation · 僅 JetStream 使用，可選自動建立 Stream
[entry_name.stream]
create   = true
name     = "STREAM_NAME"
subjects = ["pattern.*"]
storage  = "file"                 # file | memory
```

| Mode | Behaviour · 行為 |
|------|-----------------|
| `req` | Request-Reply — waits for response, prints JSON · 等待回應並輸出 JSON |
| `pub` | Publish — fire-and-forget, prints confirmation · 發後即忘，印出確認訊息 |
| `js`  | JetStream publish — prints sequence number · 發布至持久化 Stream，印出序號 |

See [`templates/`](templates/) for real-world examples · 實際範例請參閱 [`templates/`](templates/) 目錄。

---

## Variable Resolution · 變數解析

Variables are resolved in the following priority order (highest → lowest) · 變數依下列優先順序解析（高 → 低）：

```
1. CLI key=value params          ← highest · 最高優先
2. defaults in template entry
3. [functions] in config.toml    ← shell commands, cached per run · Shell 指令，單次執行快取
4. Built-in variables            ← lowest · 最低優先
```

| Built-in · 內建 | Value · 值 |
|----------------|-----------|
| `{{uuid}}`     | Random UUID v4 · 隨機 UUID v4 |
| `{{now}}`      | Unix timestamp (seconds) · Unix 時間戳（秒） |
| `{{now_ms}}`   | Unix timestamp (milliseconds) · Unix 時間戳（毫秒） |
| `{{now_iso}}`  | ISO 8601 / RFC3339 (UTC) |

> **Note · 注意：** `{{uuid}}` appearing multiple times in one body returns the **same value** (cached).  
> Use distinct names (`{{uuid2}}`, `{{uuid3}}`) for independent UUIDs.  
> 同一 body 中多次出現 `{{uuid}}` 會得到**相同值**（已快取）。  
> 需要不同 UUID 請使用不同名稱（如 `{{uuid2}}`）。

An unresolved variable causes an error on stderr and exit code 1 · 未解析的變數會輸出錯誤至 stderr 並以 exit code 1 結束。

---

## Authentication · 認證模式

| `auth_mode` | Required fields · 必填欄位 |
|-------------|--------------------------|
| `creds`     | `creds_file` |
| `token`     | `token` |
| `nkey`      | `nkey_seed_file` (`.nk` seed only — public key auto-derived · 僅需 seed 檔，公鑰自動推導) |
| `none`      | — |

---

## Examples · 範例

### Request-Reply

```bash
./nats-runner -t templates/srp_types.toml -n create \
  srp_type=ai-service-package \
  description="AI Service Package" \
  resource1=api resource2=compute \
  metric_field=aitoken
```

### Publish

```bash
./nats-runner -t templates/usage_report.toml -n submit \
  tenant_id=tenant-001 resource=compute amount=500
```

### JetStream

```bash
./nats-runner -t templates/usage_stats.toml -n record \
  tenant_id=tenant-001 stat_key=api_calls value=1200
```

### Python helper for complex payloads · Python 輔助腳本（複雜參數）

```bash
python scripts/run_with_metrics.py
```

Edit `CONFIG` inside [`scripts/run_with_metrics.py`](scripts/run_with_metrics.py) to customise the payload · 修改腳本內的 `CONFIG` 即可調整發送內容。

---

## Development · 開發

```bash
# Run all tests · 執行所有測試
go test ./...
```

### Contributing · 貢獻

This project follows a branch-based workflow · 本專案遵循分支工作流程：

| Branch · 分支 | Purpose · 用途 | Direct push |
|--------------|----------------|-------------|
| `main`       | Stable releases · 穩定發布 | ✗ PR only |
| `develop`    | Integration · 整合分支 | ✗ PR only |
| `feat/<name>` | New features · 新功能 | ✓ |
| `fix/<name>`  | Bug fixes · 錯誤修復 | ✓ |
| `chore/<name>` | Maintenance · 維護 | ✓ |

```bash
git checkout develop && git pull origin develop
git checkout -b feat/<your-feature>
# ... make changes, go test ./... passes ...
git push origin feat/<your-feature>
# Open PR targeting develop on GitHub
```

Commit messages must be in **English** and follow [Conventional Commits](https://www.conventionalcommits.org/) · commit 訊息須使用**英文**並遵循 Conventional Commits 規範。

---

## Project Structure · 專案結構

```
nats-runner.go        — Entry point · 程式進入點
internal/
  cli/                — Cobra CLI commands · CLI 指令
  config/             — TOML config loader · 設定檔載入
  domain/             — Core types (zero dependencies) · 核心型別
  nats/               — NATS connection & executors · 連線與執行器
  template/           — Template loader · 模板載入
  vars/               — Variable resolver · 變數解析引擎
templates/            — TOML template files (one per domain) · 各業務模板
configs/              — Example config files · 設定範例
scripts/              — Python helper scripts · Python 輔助腳本
```

---

## License

MIT
