# nats-runner

以 TOML 模板為驅動的 NATS 訊息發送 CLI 工具，搭配五層變數解析引擎（Go `text/template`）。
內建互動式雙欄 TUI，可在送出前即時預覽 render 後的 JSON payload。

Also available in [English](README.md).

---

## 目錄

- [功能特色](#功能特色)
- [安裝](#安裝)
- [設定](#設定)
- [使用方式](#使用方式)
- [互動模式（TUI）](#互動模式tui)
- [模板格式](#模板格式)
- [變數解析](#變數解析)
- [認證模式](#認證模式)
- [範例](#範例)
- [專案結構](#專案結構)

---

## 功能特色

- 三種 NATS 傳送模式：**Request-Reply**（`req`）、**Publish**（`pub`）、**JetStream**（`js`）
- TOML 模板驅動，新增 API 呼叫無需重新編譯
- 五層變數解析：**CLI 參數 → `--values` 檔 → 模板預設值 → Shell 函式 → 內建變數**
- Go `text/template` body：資料變數 `{{ .var }}`、內建函式 `{{ uuid }}`、pipe `{{ .x | toJson }}`
- JSON 安全：自由文字以 `{{ .field | toJson }}` 跳脫，並驗證 JSON 形態輸出是否合法
- 內建函式：`{{ uuid }}`、`{{ now }}`、`{{ now_ms }}`、`{{ now_iso }}`
- Shell 函式（`funcs/*.toml`）僅在 body 引用時執行，單次執行只跑一次
- 互動式雙欄 TUI，含即時預覽、迴圈執行器與每次執行的紀錄檔
- 認證模式：`creds`、`token`、`nkey`、`none`；支援 TLS / mTLS
- JetStream Stream 自動建立
- 單一靜態二進位，無執行期依賴

---

## 安裝

需要 Go 1.21+（module 以較新的 toolchain 為目標，詳見 `go.mod`）。

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

> 若本專案位於不含它的父層 `go.work` 內，請在 build/test 指令前加上 `GOWORK=off`。

---

## 設定

### 連線 — `configs/<name>.toml`

每個 NATS 環境一個檔，放在 `configs/`，並以**名稱**經由 `-c` 引用：

```toml
# configs/metering.toml
[connection]
url            = "nats://nats-cluster.example.com:4222"
auth_mode      = "creds"   # creds | token | nkey | none
creds_file     = "./auth/user.creds"
token          = ""
nkey_seed_file = ""
timeout_ms     = 5000

[connection.tls]            # 選填
ca_cert              = ""
client_cert          = ""
client_key           = ""
insecure_skip_verify = false
```

`-c metering` 會解析為 `configs/metering.toml`；含 `/` 的字串則視為路徑直接使用。

### 全域設定 — `~/.nats-runner.toml`

工具層級的預設值（由 `config set` 寫入）：

```toml
default_connection = "metering"
template_dir       = "templates"
funcs_dir          = "funcs"
values_dir         = "values"
```

### Shell 函式 — `funcs/<name>.toml`

每個以 Shell 指令產生的動態變數各自一個檔：

```toml
# funcs/uuid.toml
command = "uuidgen"
desc    = "Generate a random UUID v4"
```

在 body 中以 `{{ .uuid }}` 引用。函式由 `funcs_dir` 載入，**僅在被引用時執行，單次執行只跑一次**。

### config 子指令

```bash
./nats-runner config set metering          # 設定預設連線（會驗證 configs/metering.toml 是否存在）
./nats-runner config set --funcs-dir funcs --values-dir values --template-dir templates
./nats-runner config show                  # 顯示目前全域設定與連線資訊
./nats-runner config list                  # 列出 configs/ 下的連線
```

---

## 使用方式

```
nats-runner [flags] -t <模板> -n <條目> [key=value ...]
```

| Flag | 說明 |
|------|------|
| `-t` | TOML 模板檔案路徑 |
| `-n` | 模板條目名稱 |
| `-c` | 連線名稱（如 `metering`）或路徑；未指定時使用 `default_connection` |
| `--values` | values 檔（`.toml`/`.json`），可重複；後面的檔優先 |
| `--loop` | 執行次數（`0` = 無限） |
| `--interval` | 迴圈間隔（如 `60s`、`500ms`） |
| `-i` | 啟動互動式 TUI（等同 `tui` 子指令） |
| `--version` | 顯示版本 |

額外的 `key=value` 參數為最高優先變數，覆蓋其餘所有來源。
TUI 為**明確啟動**（`nats-runner tui` 或 `-i`）——不帶任何參數執行 `nats-runner`
會印出 usage 並結束，以保持對 script/自動化的友善。

---

## 互動模式（TUI）

以 `nats-runner tui`（或 `nats-runner -i`）明確啟動，進入雙欄編輯器：

- **左欄** — 可聚焦欄位：連線、模板檔、Entry，接著是「掃描 body 後得到的、該 Entry 真正需要的變數」，最後是迴圈選項與 values 檔。
- **右欄** — 即時預覽 render 後的 JSON payload（Shell 函式以 `<func:NAME>` 佔位、內建函式即時運算），並顯示 subject 與 mode。
- **底部** — 對應目前設定的等效 CLI 指令。

按鍵：`↑/↓` 在欄位間移動 · `←/→` 就地切換連線/模板/Entry（免開選單） · `Enter` 開完整選單／切換
· 在變數列直接輸入 · `Ctrl+R` 執行 · `Ctrl+C` 停止/離開。選擇模板時會自動選取第一個 Entry，
變數列即刻出現。每次執行會寫入 `logs/` 下的結構化紀錄檔。

---

## 模板格式

建議每個業務領域一個 TOML 檔，每個操作一個區段。實際範例請參閱 [`templates/`](templates/) 目錄。

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

# 僅 JetStream 使用——可選，自動建立 Stream
[entry_name.stream]
create   = true
name     = "STREAM_NAME"
subjects = ["pattern.*"]
storage  = "file"              # file | memory
```

| 模式 | 行為 |
|------|------|
| `req` | Request-Reply：等待回應，輸出格式化 JSON |
| `pub` | Publish：發後即忘，印出 `Published to: <subject>` |
| `js`  | JetStream Publish：印出 `JetStream published to: <subject> (seq: N)` |

**JSON 安全：** 自由文字欄位請以 `{{ .field | toJson }}` 包覆（會自動加引號並跳脫 `"`、`\`、換行）。
render 後若輸出形態為 JSON，工具會驗證合法性，不合法時以清楚的錯誤中止。

---

## 變數解析

body 以 Go `text/template` render。資料變數需加前綴點（`{{ .name }}`）；內建函式則不加（`{{ uuid }}`）。
解析優先順序（高 → 低）：

```
1. CLI key=value 參數              ← 最高
2. --values 檔（.toml/.json）
3. 模板條目中的 defaults
4. Shell 函式（funcs/*.toml，以 {{ .name }} 引用）
5. 內建函式                        ← 最低
```

| 內建函式 | 值 | JSON 用法 |
|----------|----|-----------|
| `{{ uuid }}`    | 隨機 UUID v4（**每次出現都產生新值**） | 字串 — 需加引號:`"{{ uuid }}"` |
| `{{ now }}`     | Unix 時間戳（秒，UTC） | 數字 — 不加引號 |
| `{{ now_ms }}`  | Unix 時間戳（毫秒，UTC） | 數字 — 不加引號 |
| `{{ now_iso }}` | ISO 8601 / RFC3339（UTC） | 字串 — 需加引號:`"{{ now_iso }}"` |

> `{{ uuid }}` 與 `{{ now_iso }}` 產生的是字串,在 JSON body 中**必須加引號**;
> 未加引號的 `{{ now_iso }}` 會產生非法 JSON。

> 內建 `{{ uuid }}` 在每個出現位置都會產生**不同值**；`{{ now }}`/`{{ now_ms }}`/`{{ now_iso }}`
> 則於單次執行擷取一次、保持一致。若想在 body 中重複使用同一個產生值，請定義 Shell 函式
> （例如 `funcs/uuid.toml`）並以 `{{ .uuid }}` 引用——函式結果於單次執行只存一份。

未解析的變數會輸出錯誤至 stderr 並以 exit code 1 結束（`missingkey=error`）。

---

## 認證模式

| `auth_mode` | 必填欄位 |
|-------------|---------|
| `creds` | `creds_file` |
| `token` | `token` |
| `nkey`  | `nkey_seed_file`（僅需 `.nk` seed 檔，公鑰自動推導） |
| `none`  | — |

---

## 範例

### Request-Reply

```bash
./nats-runner -c metering -t templates/srp_types.toml -n create \
  srp_type=ai-service-package \
  description="AI 服務包" \
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

### 迴圈 / 壓測

```bash
./nats-runner -c metering -t templates/usage_stats.toml -n record \
  tenant_id=tenant-001 --loop 100 --interval 200ms
```

### Values 檔

```bash
./nats-runner -c metering -t templates/srp_types.toml -n create_multi_metrics \
  --values values/base.toml --values values/override.json
```

---

## 專案結構

```
nats-runner.go     — 程式進入點
internal/
  cli/             — 標準 flag CLI（root、config 子指令）
  config/          — TOML 載入（連線、全域設定、funcs）
  domain/          — 核心型別（零依賴）
  nats/            — NATS 連線 + 統一 Exec（req/pub/js）
  template/        — 模板載入與條目查找
  vars/            — 基於 text/template 的五層變數解析
  logger/          — 每次執行的結構化紀錄
  tui/             — 互動式雙欄 TUI（bubbletea）
templates/         — TOML 模板（每個業務領域一個）
configs/           — 連線檔（configs/<name>.toml）
funcs/             — Shell 函式定義（funcs/<name>.toml）
values/            — 選用的 values 檔（--values）
scripts/           — Python 輔助腳本
```

---

## License

MIT
