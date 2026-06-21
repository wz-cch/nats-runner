# nats-runner Config 管理設計

> 狀態：草稿  
> 日期：2026-05-22

---

## 一、三個獨立層級

nats-runner 有三個互相獨立的配置層級。每一層有自己的管理方式：

| 層級 | 存放位置 | 用途 | 誰來修改 |
|------|-|---|---|- |
| **工具設定** | `~/.nats-runner.toml` | 程式預設值 | `nats-runner config set` |
| **連線設定** | `configs/<名稱>.toml` | NATS 連線資訊（URL、認證、TLS） | 使用者手動編輯 |
| **Shell 函數** | `funcs/<名稱>.toml` | 每個自訂變數產生命令各一檔 | 使用者新增／刪除檔案 |

這三者**互相正交**，**永遠不會嵌套**。連線設定檔裡不會有 functions，`funcs/` 裡的檔案不會是連線。

---

## 二、工具設定（`~/.nats-runner.toml`）

存在使用者家目錄裡，存放程式本身的預設值。

```toml
default_connection = "primary"    # 不傳 -c 時使用哪個連線
template_dir       = "./templates" # 模板檔案預設目錄
funcs_dir          = "./funcs"     # Shell 函數掃描目錄
values_dir         = "./values"    # Values 檔案預設目錄
```

- **不會**包含 NATS URL、認證資訊或 auth_mode。
- **不會**包含 `[functions]`。
- 用 `nats-runner config set` 子指令設定各欄位。
- 用 `nats-runner config show` 顯示。

---

## 三、連線設定（`configs/<名稱>.toml`）

`configs/` 下每一檔都是**一個 NATS 連線**。檔名 = 連線名稱。一檔一連線，檔與檔之間完全獨立。

```toml
# configs/primary.toml
[connection]
url        = "nats://dev-server:14222"
auth_mode  = "creds"
creds_file = "./auth/dev.creds"
timeout_ms = 5000
```

```toml
# configs/prod.toml
[connection]
url        = "nats://prod-server:4222"
auth_mode  = "nkey"
nkey_seed_file = "./auth/prod.nk"
timeout_ms = 10000
```

### 規則

- 相同的 NATS server 可以出現在多個設定檔中。
- 同一環境（如 dev）可以有多個連線（primary、backup 等）。
- 檔內**不**放 `[functions]`；那在 `funcs/`。
- 檔名只是名字，叫 `primary`、`backup`、`prod-us-east` 都可以。

---

## 四、Shell 函數（`funcs/<名稱>.toml`）

每個函數一個檔案。程式啟動時自動掃描 `funcs/` 下所有 `.toml` 檔案。每檔的檔名（去除 `.toml` 副檔名）成為變數的 key。

```toml
# funcs/uuid.toml
command = "uuidgen"
desc    = "產生隨機 UUID v4"
```

```toml
# funcs/now_ms.toml
command = "date +%s%3N"
desc    = "目前時間，Epoch 毫秒"
```

```toml
# funcs/random_user.toml
command = "shuf -n 1 ./data/user_list.txt"
desc    = "從名單中隨機挑使用者"
```

### 規則

- 檔名（去掉 `.toml`）= 變數 key（`{{uuid}}`、`{{now_ms}}`、`{{random_user}}`）。
- 沒有 `[functions]` 區塊。
- 沒有巢狀陣列或複雜結構。每檔只有一條指令。
- 停用方法：改名或刪除檔案。不需要 `disabled = true`。

---

## 五、CLI 使用方式

### 5.1 連線選擇

`-c` 可以是**連線名稱**或**絕對路徑**：

```bash
./nats-runner -c primary -t templates/base.srp.toml -n srp_create
# → 讀取 configs/primary.toml

./nats-runner -c /home/user/custom-config.toml -t ... -n ...
# → 讀取指定路徑檔案

./nats-runner -c prod -t ... -n ...
# → 讀取 configs/prod.toml
```

### 5.2 解析優先級

```
-c flag（名稱或路徑）          → 最高優先
~/.nats-runner.toml 的 default_connection  → 次優
報錯                            → 最低（致命錯誤）
```

### 5.3 完整命令

```bash
./nats-runner [參數] -c <連線> -t <模板> -n <條目> [key=val ...]
```

| 參數 | 說明 | 範例 |
|------|-|--|- |
| `-c <名稱\|路徑>` | 連線設定（必填，見 5.2） | `-c primary` 或 `-c ./path.toml` |
| `-t <路徑>` | 模板檔案路徑（必填） | `-t templates/base.srp.toml` |
| `-n <名稱>` | 模板條目名稱（必填） | `-n srp_create` |
| `--values <檔案>` | Values 檔（Helm 模式，選填） | `--values values/iot.json` |
| `--version` | 顯示版本 | — |
| `--help` | 顯示說明 | — |

### 5.4 Config 子指令

```bash
# 設定預設連線（若 configs/<name>.toml 不存在則報錯）
./nats-runner config set <連線名稱>

# 設定各目錄路徑
./nats-runner config set --template-dir ./templates
./nats-runner config set --funcs-dir    ./funcs
./nats-runner config set --values-dir   ./values

# 顯示目前設定
./nats-runner config show
```

`config show` 的輸出：

```
目前連線：primary（configs/primary.toml）

NATS URL：      nats://dev-server:14222
認證模式：      creds
模板目錄：      ./templates
Funcs 目錄：    ./funcs
Values 目錄：   ./values
函數：          uuid（uuidgen）, now_ms（date +%s%3N）, random_user（shuf -n 1 ...）
```

### 5.5 列出連線

```bash
./nats-runner config list
```

```
可用連線：
  primary →  configs/primary.toml     （nats://dev-server:14222）
  prod    →  configs/prod.toml        （nats://prod-server:4222）
  backup  →  configs/backup.toml      （nats://dev-server:14223）
```

---

## 六、目錄結構

```
~/.nats-runner.toml           ← 工具設定（由 `config set` 產生或手動建立）

configs/
  primary.toml                ← 連線 #1
  prod.toml                   ← 連線 #2
  backup.toml                 ← 連線 #3
  metering.toml               ← 現有連線範例檔（保留參考用）

funcs/
  uuid.toml
  now_ms.toml
  random_user.toml

values/
  iot.toml
  extra_iot.toml
  tb_package.toml
```

> `funcs_dir` 與 `values_dir` 路徑皆從 `~/.nats-runner.toml` 讀取，預設為 `./funcs` 與 `./values`（相對於執行目錄）。

---

## 七、與現有設計遷移差異

### 會有變化的

| 現有設計 | 新設計 |
|---------|-| |
| `configs/metering.toml` 同時包含 `[connection]` 和 `[functions]` | `[functions]` 拆到 `funcs/*.toml` |
| `~/.nats-runner.toml` 儲存 config 檔案的絕對路徑 | `~/.nats-runner.toml` 改存 `default_connection`（名稱） |
| `-c <路徑>` 只接受絕對路徑 | `-c <名稱>` 也支援，會自動解析 `configs/<名稱>.toml` |
| 沒有 `funcs/` 目錄 | 新增 `funcs/` 目錄專放每支函數的設定 |
| `nats-runner config path` 子指令 | 改為 `config set`／`config show`／`config list` |

### 不會變化的

- 所有 NATS 連線選項（creds、token、nkey、TLS、timeout）不變。
- `nats-runner --version` 和 `--help` 不變。
- Python 腳本**不需要修改**——它照常傳 `-c` 參數即可。

> **注意**：變數解析順序**由四層擴充為五層**：CLI > Merged Values > defaults > functions > builtins。

---

## 八、使用流程範例

### 流程 A：日常開發使用

```bash
# 第一次使用
$ ./nats-runner config set primary   # 把 "primary" 存到 ~/.nats-runner.toml

# 日常使用（不用傳 -c，讀取預設值）
$ ./nats-runner -t templates/iot_suite.toml -n srp_create SRPType=iotsuite

# 切到 prod
$ ./nats-runner -c prod -t templates/srp_types.toml -n srp_create SRPType=tb-package
```

### 流程 B：新增連線

```bash
# 只要複製檔案即可
$ cp configs/prod.toml configs/staging.toml
$ vim configs/staging.toml   # 改 url、credentials

# 直接用
$ ./nats-runner -c staging -t ...  -n ...
```

### 流程 C：新增函數

```bash
# 只要建立檔案
$ cat > funcs/day_of_week.toml << 'EOF'
command = "date +%A"
desc    = "今天星期幾"
EOF

# 就可以在模板中使用
# body = """
# {
#   "day": "{{day_of_week}}"
# }
# """
```

### 流程 D：同一環境的多條連線

```toml
# configs/dev-primary.toml
[connection]
url        = "nats://dev:4222"
auth_mode  = "none"

# configs/dev-backup.toml
[connection]
url        = "nats://dev-backup:4222"
auth_mode  = "token"
token      = "secret-token"
```

兩者都可以用，兩者連到不同的 server。不需要額外的「環境」概念。

---

## 九、Go Domain Model 變更

更新 `internal/domain/model.go`：

```go
// AppConfig 是 runtime 合併後的完整應用設定。
// Functions 不來自 TOML，由 ScanFunctions() 填入。
type AppConfig struct {
	Connection ConnectionConfig         `toml:"connection"`
	Functions  map[string]FuncConfig    // runtime only，非 TOML 欄位
}

// ConnectionConfig 純連線資訊，不含 functions。
type ConnectionConfig struct {
	URL          string    `toml:"url"`
	AuthMode     string    `toml:"auth_mode"`
	CredsFile    string    `toml:"creds_file"`
	Token        string    `toml:"token"`
	NKeySeedFile string    `toml:"nkey_seed_file"`
	TimeoutMs    int       `toml:"timeout_ms"`
	TLS          TLSConfig `toml:"tls"`
}

// GlobalConfig 是 ~/.nats-runner.toml 的工具設定。
type GlobalConfig struct {
	DefaultConnection string `toml:"default_connection"`
	TemplateDir       string `toml:"template_dir"`
	FuncsDir          string `toml:"funcs_dir"`
	ValuesDir         string `toml:"values_dir"`
}

// FuncConfig 是 funcs/*.toml 中定義的 shell 函數。
type FuncConfig struct {
	Command string `toml:"command"`
	Desc    string `toml:"desc"`
}
```

更新 `internal/config/loader.go`：

```go
// ResolveConnection 決定有效的連線設定。
// 優先順序：-c flag > default_connection > 報錯
func ResolveConnection(flagVal string) (*ConnectionConfig, string, error)
// flagVal 是使用者用 -c 傳的值
// 回傳：連線設定、來源標籤（例如 "configs/primary.toml"）

// LoadConnectionFile 讀取單一 configs/*.toml 檔案
func LoadConnectionFile(path string) (*ConnectionConfig, error)

// ScanFunctions 掃描 funcsDir 下所有 *.toml，回傳函數定義
func ScanFunctions(funcsDir string) (map[string]FuncConfig, error)
// 回傳：函數名稱 → FuncConfig{Command, Desc}
```

---

## 十、錯誤訊息

```
# 沒有指定連線
Error: 沒有指定連線。請使用 -c <名稱或路徑>，或先執行 "nats-runner config set <名稱>"。

# 連線檔案不存在（執行時）
Error: 連線 "xyz" 不存在。請執行 "nats-runner config list" 查看可用連線。

# config set 指定不存在的連線名稱
Error: 連線 "staging" 不存在（找不到 configs/staging.toml）。
       請先建立連線設定檔，再執行 "nats-runner config set staging"。

# 檔案解析錯誤
Error: 無法解析連線設定 "configs/prod.toml"（第 5 行：toml: 預期 newline 但發現 <字元>）

# 無效的 auth_mode
Error: 連線設定：auth_mode "basic" 不合法，必須是以下之一：creds、token、nkey、none

# auth_mode 必要欄位遺漏
Error: 連線設定：auth_mode 為 "creds" 時必須提供 "creds_file"

# 函數檔解析錯誤
Error: 無法載入函數 "uuid"（funcs/uuid.toml：缺少 "command" 欄位）
```
