---

# 專案開發設計文件：nats-runner

> ⚠️ **本文件為初版設計理念紀錄，部分內容已被實作演進取代。**
> 目前實際行為以 [README.md](README.md) / [AGENTS.md](AGENTS.md) / [dev-spec.md](dev-spec.md) 為準。
> 主要差異：
> - 變數語法改用 Go `text/template`：資料變數 `{{ .var }}`、內建函式 `{{ uuid }}`（不再是無點的 `{{var}}`）。
> - 解析改為**五層**：CLI > `--values` 檔 > defaults > 函式 > 內建（新增 `--values` 層）。
> - 連線以**名稱**管理（`-c <name>` → `configs/<name>.toml`）；`~/.nats-runner.toml` 為全域設定，函式移至 `funcs/*.toml`。
> - 內建 `{{ uuid }}` 每次出現都產生新值（非快取）。
> - 新增互動式雙欄 TUI、`--loop`/`--interval` 與每次執行紀錄檔。

## 1. 專案概述 (Project Overview)
`nats-runner` 是一個專為開發與運維設計的輕量級 NATS 請求執行工具。其目標是提供類似於 RESTful 環境下 Swagger 的體驗，讓使用者能在不依賴任何 Runtime（如 Python/Node.js）的跳板機環境中，透過**模組化模板**與**參數化輸入**，快速發送 NATS 訊息並獲取回覆。

### 核心設計目標
- **零依賴部署**：編譯為單一靜態二進位檔，直接 copy 到伺服器即可運行。
- **配置驅動**：所有連線、API 定義、動態變數皆由設定檔定義，無需重新編譯即可新增 API。
- **同步回饋**：支援 Request-Reply 模式，直接在終端機輸出 Response JSON。
- **極高擴展性**：透過 Shell 指令映射實現自定義動態變數生成。
- **便捷操作**：支援全域預設設定，減少重複輸入參數。

---

## 2. 系統架構 (System Architecture)

### 2.1 執行流程
`指令輸入` $\rightarrow$ `確定連線設定 (CLI $\rightarrow$ Global Config)` $\rightarrow$ `載入模板 (Template File)` $\rightarrow$ `解析並替換變數 (Priority Logic)` $\rightarrow$ `根據 Mode 執行 NATS 操作` $\rightarrow$ `輸出結果`。

### 2.2 支援模式 (Delivery Modes)
| 模式 | 關鍵字 | 行為 | 適用場景 |
| :--- | :--- | :--- | :--- |
| **Request** | `req` | 發送訊息並同步等待回覆，直到超時 | 查詢資料、建立資源 (最像 Swagger) |
| **Publish** | `pub` | 單向發送訊息，發完即結束 | 觸發通知、異步指令 |
| **JetStream**| `js` | Publish 訊息至 JetStream 持久化 Stream；預設 Stream 由接收方事先建立，亦可在模板中宣告 `[name.stream]` 區塊由工具自動建立（已存在且配置一致則略過；配置衝突則報錯中止） | 關鍵訂單、需要保證訊息不遺失的場景 |

---

## 3. 配置文件規格 (Configuration Specification)

本工具全面採用 **TOML** 格式，以解決 YAML 的縮排問題，並利用三引號 `"""` 完美支援多行 JSON 內容而無需轉義。

### 3.1 連線設定檔 (`config.toml`)
定義 NATS 集群連線資訊與自定義動態變數函數。

```toml
[connection]
url = "nats://nats-cluster.example.com:4222"
auth_mode = "creds"           # 選項: "creds" (使用 .creds 檔), "token", "nkey", "none"
creds_file = "./auth/user.creds"
token = ""                    # 當 auth_mode = "token" 時使用
nkey_seed_file = ""           # 當 auth_mode = "nkey" 時使用，填入 .nk 種子檔路徑
                              # 程式呼叫 nats.NkeyOptionFromSeed(path)，自動從種子推導公鑰並完成簽章，無需額外公鑰檔
timeout_ms = 2000             # Request 模式的超時時間

[connection.tls]              # 選填；如需 TLS 加密連線時設定（全部留空則不啟用 TLS）
ca_cert = ""                  # CA 憑證路徑（用於自簽憑證環境）
client_cert = ""              # 客戶端憑證路徑（mTLS 雙向認證）
client_key = ""               # 客戶端私鑰路徑（mTLS 雙向認證）
insecure_skip_verify = false  # 警告：設為 true 僅限本機測試，生產環境嚴禁使用

[functions]
# 自定義動態變數 $\rightarrow$ Shell 指令映射
# 程式將執行指令並將 stdout 作為變數值
unix = "date +%s"
unix_ms = "date +%s%3N"
today = "date +%Y%m%d"
uuid = "uuidgen"
random_user = "shuf -n 1 ./data/user_list.txt"
# 甚至可以對應到外部腳本
complex_logic = "/home/app/nats-tool/scripts/calc.sh"
```

### 3.2 模板定義檔 (`templates/*.toml`)
建議按模組分檔（如 `user.toml`, `order.toml`）。

```toml
[create_user]
subject = "users.create"
mode = "req"                  # 選項: "req", "pub", "js"
defaults = { role = "member", status = "active" } # 預設參數
body = """
{
  "user_id": "{{id}}",
  "username": "{{name}}",
  "role": "{{role}}",
  "status": "{{status}}",
  "created_at": "{{unix}}",
  "request_id": "{{uuid}}"
}
"""

[get_user]
subject = "users.get"
mode = "req"
body = """
{
  "user_id": "{{id}}"
}
"""

# --- JetStream 發布範例 ---

# 情境一：Stream 已由接收方事先建立，工具只負責 Publish
[publish_order]
subject = "orders.created"
mode = "js"
body = """
{
  "order_id":   "{{id}}",
  "amount":     "{{amount}}",
  "created_at": "{{unix_ms}}"
}
"""

# 情境二：由工具自動建立 Stream
# Stream 不存在 → 自動建立
# Stream 已存在且配置一致 → 略過，正常 Publish
# Stream 已存在但配置衝突（subjects/storage 等不符） → 輸出錯誤至 stderr，非零 exit code 中止
[publish_order_auto_stream]
subject = "orders.created"
mode = "js"
body = """
{
  "order_id":   "{{id}}",
  "amount":     "{{amount}}",
  "created_at": "{{unix_ms}}"
}
"""

[publish_order_auto_stream.stream]
create          = true           # true = 工具負責建立 Stream（語意詳見上方說明）
name            = "ORDERS"       # Stream 名稱（必填）
subjects        = ["orders.*"]   # 此 Stream 訂閱的主題列表（必填）
storage         = "file"         # "file"（持久化）或 "memory"（重啟後消失）
max_age_seconds = 0              # 訊息最大保留秒數；0 = 永不過期
max_msgs        = -1             # 最大訊息數量；-1 = 不限制
```

---

## 4. 變數替換系統 (Variable System)

### 4.1 替換優先級 (Priority) — 現行為五層
> 語法已改為 Go `text/template`：資料變數寫成 `{{ .var }}`（含前綴點），內建函式寫成 `{{ uuid }}`（無點）。

`body` render 時，資料變數依以下順序解析（由高到低）：
1. **CLI Params**：指令末端傳入的 `key=value` $\rightarrow$ **最高優先級**。
2. **Values 檔**：`--values` 載入的 `.toml`/`.json`（可重複，後者優先）。
3. **Template Defaults**：模板中 `defaults` 表定義的值。
4. **Shell Functions**：`funcs/*.toml` 定義的 Shell 指令輸出（以 `{{ .name }}` 引用，僅在被引用時執行）。
5. **Built-in Functions**：程式內建函式（詳見 4.2 節清單）。

### 4.2 內建函式清單 (Built-in Functions)

| 函式 | 說明 | 輸出範例 |
| :--- | :--- | :--- |
| `{{ uuid }}` | 隨機 UUID v4（**每次出現都產生新值**） | `e1c2…` |
| `{{ now }}` | 當前 Unix 時間戳（秒，UTC） | `1747526400` |
| `{{ now_ms }}` | 當前 Unix 時間戳（毫秒，UTC） | `1747526400123` |
| `{{ now_iso }}` | 當前 UTC 時間（ISO 8601） | `2026-05-18T00:00:00Z` |

亦提供 pipe helper：`{{ .arr | toJson }}`、`{{ .s | trim }}`。自由文字欄位請用 `{{ .field | toJson }}` 以確保 JSON 合法。

### 4.3 變數處理流程

> **現行規則：**
> - 內建 `{{ uuid }}`：**每次出現都產生新值**（兩處 `{{ uuid }}` 會不同）。
> - Shell 函式（`{{ .name }}`）：結果在單次 render 存一份，故同一 body 中多處 `{{ .name }}` 相同；
>   若想要「同一 UUID 重複使用」，請用 `funcs/uuid.toml` 並以 `{{ .uuid }}` 引用。
> - 函式只在 body 有引用時才執行（lazy），單次執行只跑一次。

1. 掃描 `body` 內容，收集所有 `{{var}}` 佔位符。
2. 對於每個 `{{var}}`，依 4.1 節優先級尋找對應值。
3. 若命中 `functions`，查詢快取：命中則直接使用快取值；未命中則執行 `sh -c "command"` 並將結果（去除首尾空白字元）存入快取後填入。
4. **若搜尋完四個優先級後仍無法解析該變數**，程式輸出錯誤至 stderr 並以非零 exit code 中止：
   `Error: variable "{{customer_id}}" is not defined. Provide it via CLI params, template defaults, or config functions.`
5. **若 Function 指令執行失敗**（非零 exit code），程式同樣輸出錯誤至 stderr 並中止：
   `Error: function "random_user" failed (exit 1): <stderr output>`
6. 完成所有替換後，將結果字串作為 NATS Payload 發送。

---

## 5. 全域配置管理 (Global Config)

為了避免每次執行都指定 `-c` 參數，工具支援儲存預設路徑。

- **儲存位置**：`~/.nats-runner.toml`
- **儲存內容**：`default_config_path = "/absolute/path/to/config.toml"`

### Config 子指令
- `config set <path>`：將指定設定檔路徑轉換為絕對路徑並儲存至全域設定檔。
- `config show`：顯示目前全域預設路徑，並讀取該路徑的 `config.toml` 印出連線資訊（URL, AuthMode）。

### 缺少設定檔時的行為
若執行請求指令時既未提供 `-c` 參數，`~/.nats-runner.toml` 也不存在，程式將輸出以下錯誤至 stderr 並以非零 exit code 中止：

`Error: no config file specified. Use -c <path> or run "nats-runner config set <path>" first.`

---

## 6. 指令集設計 (CLI Interface)

### 6.1 請求執行格式
```bash
./nats-runner [-c <config_path>] -t <template_path> -n <template_name> [param1=val1 param2=val2 ...]
```
- `-c`：可選。若不指定，則讀取 `~/.nats-runner.toml` 中的預設路徑。
- `-t`：必填。模板檔案路徑。若檔案不存在，輸出 `Error: template file not found: <path>` 並以非零 exit code 中止。
- `-n`：必填。模板內的 API 名稱。若名稱不存在於模板中，輸出 `Error: template "<name>" not found in <path>` 並以非零 exit code 中止。
- `params`：可選。用來覆蓋預設值或填入變數。參數值含空格時須以 Shell 引號包覆，例如 `name="John Doe"`（Shell 解引號後程式收到 `name=John Doe`，行為正確）。

### 6.2 管理指令格式
- `./nats-runner config set <path>`
- `./nats-runner config show`
- `./nats-runner --version`：顯示版本號（編譯時以 `-ldflags "-X main.version=x.y.z"` 注入）。
- `./nats-runner --help`：顯示指令用法（由 Go `flag` 套件自動生成）。

### 6.3 使用範例
```bash
# 1. 初始化全域設定
./nats-runner config set ./configs/prod.toml

# 2. 執行請求 (使用預設 config，填入 id 與 name)
./nats-runner -t templates/user.toml -n create_user id=101 name=Jack

# 3. 執行請求 (覆蓋預設 role 為 admin)
./nats-runner -t templates/user.toml -n create_user id=102 name=Boss role=admin

# 4. 參數值含空格時以 Shell 引號包覆（引號由 Shell 解除，不會傳入程式內部）
./nats-runner -t templates/user.toml -n create_user id=103 name="John Doe" role=member

# 5. 查看目前環境設定
./nats-runner config show
```

---

## 7. 技術實作指南 (Implementation Guide)

### 7.1 開發要求
- **語言**：Go (Golang) $\rightarrow$ 編譯為靜態二進位檔 (`CGO_ENABLED=0`)。
- **關鍵套件**：
    - `github.com/nats-io/nats.go`（含 JetStream API `js, _ := nc.JetStream()`）
    - `github.com/BurntSushi/toml`
    - `os/exec`（用於執行 Shell Functions）
- **跨平台編譯**：
    ```bash
    # Linux amd64（最常見的伺服器環境）
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=1.0.0" -o nats-runner .
    # Linux arm64（ARM 伺服器）
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-X main.version=1.0.0" -o nats-runner-arm64 .
    ```

### 7.2 實作細節
- **認證**：根據 `auth_mode` 決定使用 `nats.UserCredentials(path)`、`nats.Token(token)` 或 `nats.NkeyOptionFromSeed(path)`；TLS 設定若存在則附加 `nats.RootCAs()`、`nats.ClientCert()` 等選項。
- **Shell 執行**：必須使用 `exec.Command("sh", "-c", cmdString)` 以支援複雜的 Linux 命令（如 Pipe, awk）。
- **輸出處理**（正常結果輸出至 stdout，所有錯誤輸出至 stderr）：
    - `mode == "req"` $\rightarrow$ 將 `msg.Data` 嘗試格式化為 Pretty-Print JSON 後輸出；若非合法 JSON 則直接輸出原始字串。
    - `mode == "pub"` $\rightarrow$ 輸出 `Published to: <subject>`。
    - `mode == "js"` $\rightarrow$ 呼叫 `js.Publish()` 同步等待 Server Ack；成功時輸出 `JetStream published to: <subject> (seq: <ack.Sequence>)`（`seq` 為 Server 回傳的 Ack 序號）。若 Ack 失敗（Stream 不存在、連線逾時、配置衝突等），輸出錯誤至 stderr 並以非零 exit code 中止。

---

## 8. 部署與維護 (Deployment & Maintenance)

### 8.1 部署步驟
1. 上傳 `nats-runner` 二進位檔。
2. 建立 `configs/` 與 `templates/` 資料夾並上傳對應 TOML 檔。
3. 執行 `./nats-runner config set <path>` 完成初始化。

### 8.2 維護流程
- **新增 API** $\rightarrow$ 在 `templates/*.toml` 中增加一個 `[name]` 區塊 $\rightarrow$ **立即生效**。
- **增加動態變數** $\rightarrow$ 在 `config.toml` 的 `[functions]` 中增加一條 Shell 指令 $\rightarrow$ **立即生效**。
- **切換環境** $\rightarrow$ 使用 `config set` 切換至不同環境的 `config.toml`。