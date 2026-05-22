# nats-runner

零依賴的 NATS 訊息發送 CLI 工具，以 TOML 模板為驅動，搭配四層變數解析引擎。

Also available in [English](README.md).

---

## 目錄

- [功能特色](#功能特色)
- [安裝](#安裝)
- [設定](#設定)
- [使用方式](#使用方式)
- [模板格式](#模板格式)
- [變數解析](#變數解析)
- [認證模式](#認證模式)
- [範例](#範例)
- [專案結構](#專案結構)

---

## 功能特色

- 三種 NATS 傳送模式：**Request-Reply**、**Publish**、**JetStream**
- TOML 模板驅動，新增 API 呼叫無需重新編譯
- 四層變數解析：CLI 參數 → 模板預設值 → Shell 函式 → 內建變數
- 內建變數：`{{uuid}}`、`{{now}}`、`{{now_ms}}`、`{{now_iso}}`
- Shell 函式整合，執行結果於單次執行期間快取
- 認證模式：`creds`、`token`、`nkey`、`none`
- TLS 支援：CA 憑證、mTLS、略過驗證
- JetStream Stream 自動建立
- 單一靜態二進位，無執行期依賴

---

## 安裝

需要 Go 1.21+

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

## 設定

在 `~/.nats-runner.toml` 建立設定檔（或以 `-c` 指定路徑）：

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
uuid = "uuidgen"   # Shell 指令，結果依變數名稱快取於單次執行中
```

完整範例請參考 [`configs/metering.toml`](configs/metering.toml)。

### 管理設定檔路徑

```bash
./nats-runner config set /path/to/config.toml   # 儲存為全域預設路徑
./nats-runner config show                        # 顯示目前預設路徑
```

---

## 使用方式

```
nats-runner [flags] -t <模板> -n <條目> [key=value ...]
```

| Flag | 說明 | 必填 |
|------|------|------|
| `-t` | TOML 模板檔案路徑 | ✓ |
| `-n` | 模板條目名稱 | ✓ |
| `-c` | 設定檔路徑（覆蓋預設） | — |
| `--version` | 顯示版本 | — |

額外的 `key=value` 參數會作為變數傳入，並覆蓋模板預設值。

---

## 模板格式

建議每個業務領域一個 TOML 檔，每個 API 操作一個區段。實際範例請參閱 [`templates/`](templates/) 目錄。

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
| `pub` | Publish：發後即忘，印出確認訊息 |
| `js`  | JetStream Publish：發布至持久化 Stream，印出序號 |

---

## 變數解析

`{{name}}` 佔位符依以下優先順序解析（高 → 低）：

```
1. CLI key=value 參數
2. 模板條目中的 defaults
3. config.toml 的 [functions]（Shell 指令，結果快取於單次執行）
4. 內建變數
```

| 內建變數 | 值 |
|----------|----|
| `{{uuid}}`    | 隨機 UUID v4 |
| `{{now}}`     | Unix 時間戳（秒） |
| `{{now_ms}}`  | Unix 時間戳（毫秒） |
| `{{now_iso}}` | ISO 8601 / RFC3339（UTC） |

> 同一 body 中多次出現 `{{uuid}}` 會得到**相同值**（單次執行快取）。  
> 需要不同 UUID 請使用不同名稱，例如 `{{uuid2}}`、`{{uuid3}}`。

未解析的變數會輸出錯誤至 stderr 並以 exit code 1 結束。

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
./nats-runner -t templates/srp_types.toml -n create \
  srp_type=ai-service-package \
  description="AI 服務包" \
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

### Python 輔助腳本（複雜參數）

```bash
python scripts/run_with_metrics.py
```

修改 [`scripts/run_with_metrics.py`](scripts/run_with_metrics.py) 內的 `CONFIG` 即可調整發送內容。

---

## 專案結構

```
nats-runner.go     — 程式進入點
internal/
  cli/             — CLI 指令（root、config）
  config/          — TOML 設定檔載入
  domain/          — 核心型別（零依賴）
  nats/            — NATS 連線與執行器
  template/        — 模板載入
  vars/            — 變數解析引擎
templates/         — TOML 模板（每個業務領域一個）
configs/           — 設定範例
scripts/           — Python 輔助腳本
```

---

## License

MIT
