# nats-runner 開發設計規格 (dev-spec)

> 版本：1.0  
> 日期：2026-05-22  
> 依據：config-design.md、values-design.md、interactive-design.md

---

## 一、開發階段總覽

三個需求依依賴關係分為四個階段，**每個階段完成後必須通過 `go test ./...`**，才能進入下一階段。

| 階段 | 名稱 | 核心目標 | 主要套件 |
|------|------|---------|---------|
| **P0** | 設定層重構 | 拆解 config 結構，建立新 domain model | `domain/`、`config/`、`cli/` |
| **P1** | 變數引擎升級 | 引入 Go `text/template`、Values 檔載入、五層解析 | `vars/`、`template/` |
| **P2** | Loop 執行器 + Log | 新增 `--loop`/`--interval` CLI 旗標、Log 寫入 | `cli/`、`internal/logger/` |
| **P3** | TUI 互動模式 | bubbletea 五畫面 TUI | `internal/tui/` |

---

## 二、現有程式碼差距分析

| 檔案 | 目前狀態 | 需要變更 |
|------|---------|---------|
| `internal/domain/model.go` | `AppConfig.Functions` 為 `map[string]string` toml 欄位；`GlobalConfig` 只有 `DefaultConfigPath` | **重寫**：新增 `FuncConfig`、更新 `GlobalConfig`、`AppConfig.Functions` 改為 runtime 欄位 |
| `internal/config/loader.go` | `ResolveConfigPath` 回傳路徑字串；`LoadAppConfig` 讀整包；無 `ScanFunctions` | **重寫**：`ResolveConnection`、`LoadConnectionFile`、`ScanFunctions` |
| `internal/cli/root.go` | 只有 `-c`、`-t`、`-n`、`--version`；無 `--values`、`--loop`、`--interval` | **新增旗標**；更新 config 解析邏輯 |
| `internal/cli/config_cmd.go` | 只有 `set <path>`（絕對路徑）與 `show` | **重寫**：`set <name>`（驗證連線存在）、`set --*-dir`、`list` |
| `internal/vars/resolver.go` | Regex 替換引擎，`{{key}}` 語法；`Resolve` 接受 `functions map[string]string` | **重寫**：Go `text/template` 引擎；更新 `Resolve` 簽章 |
| `internal/template/loader.go` | 讀取 `TemplateEntry`，無多模板掃描 | **新增** `ScanTemplates`；`TemplateEntry.Defaults` 型別不變 |
| `configs/metering.toml` | 同時含 `[connection]` 與 `[functions]` | 遷移：移除 `[functions]`，改放 `funcs/` |

---

## 三、P0 — 設定層重構

### 3.1 `internal/domain/model.go` 完整重寫

```go
package domain

// AppConfig 是 runtime 合併後的完整應用設定。
// Functions 欄位不來自 TOML，由 config.ScanFunctions() 在啟動時填入。
type AppConfig struct {
    Connection ConnectionConfig      `toml:"connection"`
    Functions  map[string]FuncConfig // runtime only — 無 toml tag
}

// ConnectionConfig 純連線資訊，對應 configs/<name>.toml 的 [connection] section。
type ConnectionConfig struct {
    URL          string    `toml:"url"`
    AuthMode     string    `toml:"auth_mode"`   // "creds" | "token" | "nkey" | "none"
    CredsFile    string    `toml:"creds_file"`
    Token        string    `toml:"token"`
    NKeySeedFile string    `toml:"nkey_seed_file"`
    TimeoutMs    int       `toml:"timeout_ms"`
    TLS          TLSConfig `toml:"tls"`
}

// TLSConfig — 不變，保留現有結構。
type TLSConfig struct {
    CACert             string `toml:"ca_cert"`
    ClientCert         string `toml:"client_cert"`
    ClientKey          string `toml:"client_key"`
    InsecureSkipVerify bool   `toml:"insecure_skip_verify"`
}

// GlobalConfig 對應 ~/.nats-runner.toml。
// Breaking change：欄位名稱從 default_config_path → default_connection。
type GlobalConfig struct {
    DefaultConnection string `toml:"default_connection"`
    TemplateDir       string `toml:"template_dir"`
    FuncsDir          string `toml:"funcs_dir"`
    ValuesDir         string `toml:"values_dir"`
}

// FuncConfig 對應 funcs/<name>.toml。
type FuncConfig struct {
    Command string `toml:"command"`
    Desc    string `toml:"desc"`
}

// TemplateEntry — 不變，保留現有結構。
type TemplateEntry struct {
    Subject  string            `toml:"subject"`
    Mode     string            `toml:"mode"`
    Defaults map[string]string `toml:"defaults"` // 純靜態字串，不渲染
    Body     string            `toml:"body"`
    Stream   *StreamConfig     `toml:"stream"`
}

// StreamConfig — 不變。
type StreamConfig struct {
    Create        bool     `toml:"create"`
    Name          string   `toml:"name"`
    Subjects      []string `toml:"subjects"`
    Storage       string   `toml:"storage"`
    MaxAgeSeconds int64    `toml:"max_age_seconds"`
    MaxMsgs       int64    `toml:"max_msgs"`
}
```

**測試**：`domain/` 無邏輯，不需要測試，但確保其他套件在修改後仍能編譯通過。

---

### 3.2 `internal/config/loader.go` 完整重寫

新 API 契約（舊 `ResolveConfigPath`、`LoadAppConfig`、`SaveGlobalConfig` 廢除或改名）：

```go
package config

// ─── 連線相關 ────────────────────────────────────────────────────

// ResolveConnection 依以下優先順序決定有效連線：
//   1. flagVal 非空 → 當成連線名稱（查 configs/<flagVal>.toml）或絕對路徑
//   2. GlobalConfig.DefaultConnection → 查 configs/<name>.toml
//   3. 兩者皆空 → 回傳錯誤
// 回傳：ConnectionConfig 與來源標籤（用於 config show 顯示）
func ResolveConnection(flagVal string) (*domain.ConnectionConfig, string, error)

// LoadConnectionFile 讀取單一 configs/*.toml，解碼 [connection] section。
// 驗證 auth_mode 合法性與必要欄位（見 3.4 驗證規則）。
func LoadConnectionFile(path string) (*domain.ConnectionConfig, error)

// resolveConnPath 內部函數：
//   - 若 name 含 "/" 或 "\" → 視為路徑直接使用
//   - 否則 → 拼接 configs/<name>.toml（相對於 cwd）
func resolveConnPath(name string) string

// ─── GlobalConfig ────────────────────────────────────────────────

// LoadGlobalConfig 讀取 ~/.nats-runner.toml。
// 若檔案不存在 → 回傳空 GlobalConfig（不報錯，使用者可能尚未設定）。
func LoadGlobalConfig() (*domain.GlobalConfig, error)

// SaveGlobalConfig 寫入 ~/.nats-runner.toml（mode 0600）。
// 只寫入非空欄位，不覆蓋未變更的欄位（merge 既有內容）。
func SaveGlobalConfig(gc *domain.GlobalConfig) error

// globalConfigPath 回傳 ~/.nats-runner.toml 絕對路徑。
func globalConfigPath() (string, error)

// ─── Shell Functions ─────────────────────────────────────────────

// ScanFunctions 掃描 funcsDir 下所有 *.toml，以檔名（無副檔名）為 key。
// 若 funcsDir 不存在 → 回傳空 map（不報錯，funcs/ 為可選）。
// 若某檔案缺少 command 欄位 → 回傳描述性錯誤，終止掃描。
func ScanFunctions(funcsDir string) (map[string]domain.FuncConfig, error)

// ─── 連線列表 ────────────────────────────────────────────────────

// ListConnections 掃描 configs/ 目錄，回傳所有連線名稱與其 URL。
// 用於 config list 指令。
func ListConnections(configsDir string) ([]ConnectionInfo, error)

type ConnectionInfo struct {
    Name string
    Path string
    URL  string
}
```

**`auth_mode` 驗證規則**（在 `LoadConnectionFile` 中實作）：

| auth_mode | 必要欄位 | 可選欄位 |
|-----------|---------|---------|
| `creds` | `creds_file` | — |
| `token` | `token` | — |
| `nkey` | `nkey_seed_file` | — |
| `none` | （無） | — |
| 其他 | — | → 報錯 |

**測試案例**：
- `ResolveConnection("")` 且 `~/.nats-runner.toml` 無 default → 回傳錯誤
- `ResolveConnection("primary")` 且 `configs/primary.toml` 不存在 → 回傳錯誤
- `LoadConnectionFile` 正常路徑
- `LoadConnectionFile` `auth_mode = "creds"` 但無 `creds_file` → 報錯
- `ScanFunctions` 目錄不存在 → 空 map，無錯誤
- `ScanFunctions` 某檔缺 `command` → 報錯
- `SaveGlobalConfig` merge 既有欄位（不清空未傳的欄位）

---

### 3.3 `internal/cli/config_cmd.go` 重寫

新的 `handleConfigCmd` 支援三個子指令：

```
nats-runner config set <連線名稱>
nats-runner config set --template-dir <路徑>
nats-runner config set --funcs-dir <路徑>
nats-runner config set --values-dir <路徑>
nats-runner config show
nats-runner config list
```

**`config set <連線名稱>` 流程**：
1. 呼叫 `resolveConnPath(name)` 取得檔案路徑
2. 呼叫 `os.Stat(path)`：若不存在 → 立即報錯（不寫入 global config）
3. 讀取 `LoadGlobalConfig()`（允許不存在）
4. 更新 `gc.DefaultConnection = name`
5. 呼叫 `SaveGlobalConfig(gc)` 寫回（保留其他欄位）

**`config set --*-dir` 流程**：
1. 解析 flag（使用 `flag.FlagSet`）
2. 讀取既有 `GlobalConfig`
3. 更新對應欄位
4. 寫回

**`config show` 輸出格式**（參考 config-design.md §5.4）：
```
目前連線：primary（configs/primary.toml）

NATS URL：      nats://dev-server:14222
認證模式：      creds
模板目錄：      ./templates
Funcs 目錄：    ./funcs
Values 目錄：   ./values
函數：          uuid（uuidgen）, now_ms（date +%s%3N）
```

**`config list` 輸出格式**：
```
可用連線：
  primary → configs/primary.toml  （nats://dev-server:14222）
  prod    → configs/prod.toml     （nats://prod-server:4222）
```

---

### 3.4 `internal/cli/root.go` 新增旗標

在現有 `flag.FlagSet` 中新增：

```go
// 已存在
configPath  := fs.String("c", "", "連線名稱或設定檔路徑")
templatePath := fs.String("t", "", "模板 TOML 檔案路徑")
templateName := fs.String("n", "", "模板條目名稱")
showVersion  := fs.Bool("version", false, "顯示版本")

// 新增
valuesFiles  := fs.StringArray("values", nil, "Values 檔（可重複）")  // 需自訂 StringArray type
loopCount    := fs.Int("loop", 1, "執行次數（0=無限）")
loopInterval := fs.String("interval", "0", "Loop 間隔（如 60s、500ms）")
```

> `StringArray` 需自訂（Go 標準 `flag` 不支援重複旗標），或改用 `github.com/spf13/pflag`。推薦**自訂 `multiStringFlag`**（避免新依賴）：
>
> ```go
> type multiStringFlag []string
> func (f *multiStringFlag) String() string  { return strings.Join(*f, ",") }
> func (f *multiStringFlag) Set(v string) error { *f = append(*f, v); return nil }
> ```

**`loopInterval` 解析**：使用 `time.ParseDuration`（接受 `60s`、`500ms`、`1m`）。`0` 或空字串表示無間隔。

**新的執行流程**（`Execute` 函數）：
1. 解析旗標
2. `ResolveConnection(*configPath)` → 取得 `ConnConfig` 與來源標籤
3. `ScanFunctions(gc.FuncsDir)` → 取得 `map[string]FuncConfig`
4. 載入 template entry
5. `LoadValuesFiles(*valuesFiles)` → 取得 `map[string]any`（P1 新增）
6. 建立 `vars.ResolveContext`
7. 開始 Loop（P2 新增）

---

## 四、P1 — 變數引擎升級

### 4.1 `internal/vars/values_loader.go`（新檔）

```go
package vars

// LoadValuesFiles 依序載入 paths 中的值檔，後者覆蓋前者。
// 支援 .toml 與 .json 兩種格式（由副檔名自動判斷）。
// 所有值統一存為 map[string]any，以支援陣列與巢狀物件。
func LoadValuesFiles(paths []string) (map[string]any, error)

// loadTOML 讀取單一 .toml 值檔
func loadTOML(path string) (map[string]any, error)

// loadJSON 讀取單一 .json 值檔
func loadJSON(path string) (map[string]any, error)

// mergeMaps 將 src 合併入 dst（src 優先）
func mergeMaps(dst, src map[string]any)
```

**格式範例**：
```toml
# values/iot.toml
srp_type    = "iotsuite"
description = "IoT Suite"
resources   = []
metrics = [
  { field = "tag",       type = "sum" },
  { field = "dashboard", type = "sum" }
]
```

```json
// values/override.json
{
  "srp_type": "custom",
  "metrics": [{ "field": "cpu", "type": "avg" }]
}
```

**測試案例**：
- 空 paths → 回傳空 map
- 單一 .toml 正常解析
- 單一 .json 正常解析
- 多檔合併：後者覆蓋前者
- 不存在的檔案 → 報錯
- 副檔名不支援 → 報錯
- 巢狀陣列/物件正確保留型別

---

### 4.2 `internal/vars/resolver.go` 完整重寫

**核心設計：Go `text/template` 引擎取代 regex**

```go
// ResolveContext 封裝五層解析所需的所有資料。
type ResolveContext struct {
    CLIParams  map[string]string         // 最高優先
    MergedVals map[string]any            // --values 合併結果
    Defaults   map[string]string         // 模板 entry.Defaults（純靜態）
    Functions  map[string]domain.FuncConfig // funcs/*.toml
    // builtins 以 FuncMap 形式注入，不在此結構中
}

// Resolve 將 body 以 Go text/template 引擎渲染。
// 資料優先順序（由低至高）：
//   builtins (FuncMap) < functions (shell, cached) < defaults < mergedVals < cliParams
// 回傳：渲染後的字串，或首個遇到的錯誤。
func Resolve(body string, ctx ResolveContext) (string, error)
```

**`Resolve` 內部流程**：

```
1. buildDataMap(ctx) → map[string]any
   ├─ a. 執行所有 Functions 的 shell 指令，結果存入 data（cached）
   ├─ b. 疊加 Defaults（string → any）
   ├─ c. 疊加 MergedVals
   └─ d. 疊加 CLIParams（最高優先，覆蓋所有 any 型別）

2. buildFuncMap() → template.FuncMap
   ├─ "uuid"     → func() string { return newUUID() }
   ├─ "now"      → func() string { return strconv.FormatInt(now.Unix(), 10) }
   ├─ "now_ms"   → func() string { return strconv.FormatInt(now.UnixMilli(), 10) }
   ├─ "now_iso"  → func() string { return now.Format(time.RFC3339) }
   ├─ "toJson"   → func(v any) (string, error) { ... json.Marshal }
   └─ "trim"     → strings.TrimSpace

3. template.New("body").Funcs(funcMap).Parse(body)

4. tmpl.Execute(buf, data)  // data 以 . 存取

5. 回傳 buf.String()
```

**Builtin 語法規則**：
- 內建變數為 **FuncMap 函數**，語法為 `{{ uuid }}`（無 `.`，每次 `Execute` 呼叫一次）
- 資料變數（values/defaults/funcs/cli）為 **data map 中的 key**，語法為 `{{ .srp_type }}`
- Shell function 結果在 `buildDataMap` 時預先執行並 cache，塞入 data map → 語法也是 `{{ .func_name }}`

> 注意：`{{ uuid }}` 每次 `Execute` 都會產生**新的** UUID。若需同一次 render 內的 uuid 一致，改用 `{{ .uuid }}`（預先 cache 放入 data）。設計決策：**維持現有行為（每次呼叫皆產生新值）**；若需 cache，使用者可透過 CLI 傳入 `uuid=<val>`。

**`toJson` 實作**：
```go
func toJsonFn(v any) (string, error) {
    b, err := json.Marshal(v)
    if err != nil {
        return "", fmt.Errorf("toJson: %w", err)
    }
    return string(b), nil
}
```

**`buildDataMap` 注意事項**：
- `map[string]string` 的值需轉換為 `any` 才能存入統一的 `map[string]any`
- CLI `key=val` 覆蓋時，即使 merged values 中該 key 是 `[]any`，也會被字串覆蓋（預期行為）
- Shell function 執行使用現有的 `runShellFn`（保持可 mock）

**測試案例**：
- 五層優先順序正確（每層覆蓋較低層）
- `{{ .missing_var }}` → Go template 預設輸出空字串 `<no value>`；**應視為錯誤**，需在 `Execute` 後掃描輸出或使用 `Option("missingkey=error")`
- `{{ uuid }}` 每次呼叫均不同
- `{{ .metrics | toJson }}` 正確序列化陣列
- `{{ if .resources }}...{{ end }}` 條件分支正確
- Shell function 失敗 → 報錯中斷
- body 含語法錯誤 → `Parse` 階段報錯

> **`missingkey` 策略**：呼叫 `tmpl.Option("missingkey=error")` 讓 Go template 在遇到不存在的 `.key` 時立即報錯（取代現有「unresolved variable」錯誤機制）。

---

### 4.3 `internal/template/loader.go` 新增 `ScanTemplates`

```go
// ScanTemplates 掃描 dir 下所有 *.toml，回傳每個檔案的名稱與 entry 數量。
// 用於 TUI 畫面 2 顯示模板列表。
// 不完整解析 body，只計算 entry 數量（TOML top-level key 數）。
func ScanTemplates(dir string) ([]TemplateFileInfo, error)

type TemplateFileInfo struct {
    FileName   string // 如 "base.srp.toml"
    Path       string // 完整路徑
    EntryCount int    // 條目數量
}
```

---

## 五、P2 — Loop 執行器 + Log

### 5.1 `internal/logger/logger.go`（新套件）

```go
package logger

// Logger 管理單次執行期間的 Log 檔案。
// 檔名格式：logs/nats-runner-YYYYMMDD_HHMMSS.log
type Logger struct {
    path string
    file *os.File
    mu   sync.Mutex
}

// New 建立並開啟 Log 檔（自動建立 logs/ 目錄）。
func New() (*Logger, error)

// Close 關閉檔案。
func (l *Logger) Close() error

// Path 回傳 Log 檔絕對路徑（用於 TUI 顯示）。
func (l *Logger) Path() string

// WriteEntry 寫入一筆執行記錄（goroutine-safe）。
func (l *Logger) WriteEntry(e Entry)

// Entry 代表一次 NATS 操作的完整記錄。
type Entry struct {
    Timestamp  time.Time
    Action     string            // "[REQ]" | "[PUB]" | "[JS]"
    Subject    string
    Values     map[string]any    // 實際傳送的變數值
    RequestBody string
    Reply      string
    DurationMs float64
    Error      error
}
```

**Log 格式**（純文字，人類可讀）：
```
[TIMESTAMP] 2026-05-22 16:00:01
[ACTION]    [REQ] nats://primary/eco1j.infra.metering.srp-types.create
[VALUES]    srp_type="iotsuite" description="IoT Suite"
[REQUEST]   {"reqSeqId":"abc-123","timestamp":1748000401000,"data":{...}}
[REPLY]     {"status":"ok","id":"req-001"}
[DURATION]  12.5ms
---
```

若有錯誤：
```
[ERROR]     NATS: timeout after 5000ms
```

---

### 5.2 Loop 執行邏輯（`internal/cli/root.go` 更新）

```go
// execLoop 執行 count 次操作，每次間隔 interval。
// count == 0 → 無限執行直到 ctx 被取消。
// 每次執行完畢呼叫 onResult 回呼（用於 TUI 即時更新）。
func execLoop(
    ctx context.Context,
    nc nats.Conn,
    entry *domain.TemplateEntry,
    resolveCtx vars.ResolveContext,
    count int,
    interval time.Duration,
    logger *logger.Logger,
    onResult func(result LoopResult),
) error

// LoopResult 代表單次迴圈的執行結果。
type LoopResult struct {
    Iteration  int
    DurationMs float64
    Reply      string  // 截斷至 200 字元（TUI 顯示用）
    Err        error
}
```

**CLI 執行模式（非 TUI）**：
- `onResult` 直接 `fmt.Println` 到 stdout
- 透過 `signal.NotifyContext(context.Background(), os.Interrupt)` 捕捉 Ctrl+C
- 結束後印出 Log 路徑

**注意**：每次迭代都要**重新呼叫 `vars.Resolve`**（因為 `{{ uuid }}` 與 `{{ now_ms }}` 需要每次重新求值）。

---

## 六、P3 — TUI 互動模式

### 6.1 依賴套件

```
github.com/charmbracelet/bubbletea   v0.27+
github.com/charmbracelet/bubbles     v0.20+   （list, textinput, table, filepicker）
github.com/charmbracelet/lipgloss    v1.0+
```

在 `go.mod` 新增，執行 `go mod tidy`。

---

### 6.2 套件結構

```
internal/tui/
  model.go           — 頂層 Model、Screen 常數、Update dispatcher
  screen_global.go   — 畫面 1：連線選擇（持久設定）
  screen_template.go — 畫面 2：選擇模板
  screen_values.go   — 畫面 3：值檔順序 + Loop 設定   [新增]
  screen_vars.go     — 畫面 4：變數總覽與覆蓋
  screen_entry.go    — 畫面 5：選擇 Entry
  screen_exec.go     — 畫面 6：執行監控
  smart_edit.go      — 結構化 JSON 編輯元件
  filepicker.go      — 檔案瀏覽器包裝
  styles.go          — lipgloss 樣式定義（UI 文字一律使用正體中文）
  strings.go         — TUI 正體中文字串常數  [新增]
  run.go             — TUI 進入點（供 cli/ 呼叫）
```

---

### 6.3 `model.go` — 頂層 Model

```go
package tui

import tea "github.com/charmbracelet/bubbletea"

// Screen 代表目前所處的畫面。
type Screen int

const (
    ScreenGlobal   Screen = iota // 畫面 1：連線選擇
    ScreenTemplate               // 畫面 2：選擇模板
    ScreenValues                 // 畫面 3：值檔 + Loop  [新增]
    ScreenVars                   // 畫面 4：變數總覽
    ScreenEntry                  // 畫面 5：選擇 Entry
    ScreenExec                   // 畫面 6：執行監控
)

// Model 是整個 TUI 的頂層狀態。
type Model struct {
    screen Screen

    // 持久設定（畫面 1 — Connection）
    globalCfg    domain.GlobalConfig
    connList     []config.ConnectionInfo // 掃描 configs/ 得到
    selectedConn string

    // 執行流程狀態（畫面 2 — Template）
    templateFiles []template.TemplateFileInfo
    selectedTmpl  string // 模板檔案路徑
    allEntries    map[string]domain.TemplateEntry

    // 執行流程狀態（畫面 3 — Values + Loop，由 screen_values 設定）
    valuesList   []string      // 已選的 values 檔，按優先順序
    loopEnabled  bool
    loopCount    int
    loopInterval time.Duration

    // 執行流程狀態（畫面 4 — Variables）
    varOverrides map[string]string // 使用者手動覆蓋的值
    mergedVars   map[string]any    // 合併後的完整變數表
    varSources   map[string]string // key → "cli"|"values"|"defaults"|"func"|"builtin"

    // 執行流程狀態（畫面 5 — Entry）
    selectedEntry string

    // 子畫面 models（bubbles 元件）
    globalModel   globalModel
    templateModel templateModel
    valuesModel   valuesModel   // 新增：畫面 3
    varsModel     varsModel
    entryModel    entryModel
    execModel     execModel

    // 共用
    width, height int
    err           error
}

// Init 初始化：讀取 GlobalConfig、掃描連線列表。
func (m Model) Init() tea.Cmd

// Update 根據目前 screen 分派給對應的子 Update。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)

// View 根據目前 screen 呼叫對應的子 View。
func (m Model) View() string
```

---

### 6.4 `screen_global.go` — 畫面 1：連線選擇

> 職責縮小為只管理「使用哪個連線」。Values 與 Loop 設定移至畫面 3（`screen_values.go`）。

**狀態**：
```go
type globalModel struct {
    connList  list.Model // 掃描 configs/ 得到的連線列表
    statusMsg string     // 暫時顯示的回饋訊息（如 "已設為預設連線"，1 秒後消失）
}
```

**按鍵處理**：
| 按鍵 | 動作 |
|------|------|
| `↑`/`↓`/`j`/`k` | 移動游標 |
| `g`/`G` | 跳至頭/尾 |
| `s` | 儲存選中連線為預設，顯示確認訊息 |
| `Enter` | 確認選擇連線 → 進入畫面 2（選模板） |
| `Esc` | no-op（畫面 1 無上層） |
| `q` | 退出 TUI |

**儲存流程（按 `s`）**：
1. `config.LoadGlobalConfig()` → gc
2. 更新 `gc.DefaultConnection = selectedConn`
3. `config.SaveGlobalConfig(gc)` 寫回（保留其他欄位）
4. 顯示確認訊息 "已設為預設連線"（1 秒後消失）

---

### 6.5 `screen_values.go` — 畫面 3：值檔 + Loop 設定（新增）

此畫面在使用者選定模板後進入，讓使用者針對**該模板**選擇適合的值檔並設定迴圈參數。

**狀態**：
```go
// 畫面 3：值檔順序 + Loop 設定
// 此畫面在選定模板後才進入，values 設定與模板上下文綁定。
type valuesModel struct {
    // Values 來源
    valDirInput   textinput.Model  // 預設目錄輸入欄（來自 GlobalConfig.ValuesDir）
    valFileList   list.Model       // 已選值檔，按優先順序排列
    valFilePicker *filepicker.Model // 按 a 開啟

    // Loop 設定
    loopOn            bool
    loopCountInput    textinput.Model
    loopIntervalInput textinput.Model
    loopUnit          string // "s" | "ms" | "min"，預設 "s"

    // 焦點管理
    focusSection int // 0=Values目錄, 1=Values清單, 2=Loop次數, 3=Loop間隔
}
```

**按鍵處理**：
| 按鍵 | 適用範圍 | 動作 |
|------|---------|------|
| `↑`/`↓`/`j`/`k` | Values 清單 | 移動游標 |
| `Tab` | 整體 | 切換 section（Values目錄 → Values清單 → Loop次數 → Loop間隔） |
| `Enter` | Values目錄輸入欄 | 開啟 filepicker |
| `a` | Values清單 focused | 開啟 filepicker 新增值檔 |
| `d` | Values清單 focused | 移除選中值檔 |
| `+`/`K` | Values清單 focused | 上移（提高優先順序） |
| `-`/`J` | Values清單 focused | 下移（降低優先順序） |
| `Space` | Loop 區塊 | 切換 Loop ON/OFF |
| `u` | Loop 間隔 focused | 循環切換單位：`s` → `ms` → `min` → `s` |
| `Enter` | 整體（確認） | 觸發 `loadValuesCmd` → 進入畫面 4（變數總覽） |
| `Esc` | filepicker 開啟 | 關閉 filepicker |
| `Esc` | 一般狀態 | 返回畫面 2（選模板） |

> **單位標籤顯示**：Loop 間隔輸入欄右側顯示當前單位，如：`[ 60 ] s`。按 `u` 切換後立即更新：`[ 60 ] ms`、`[ 60 ] min`。

**`loadValuesCmd` 觸發時機**（按 Enter 確認，進入畫面 4 前）：
1. 將 `loopIntervalInput.Value()` 與 `loopUnit` 組合成 `time.Duration`
   - `"s"` → `time.ParseDuration(val + "s")`
   - `"ms"` → `time.ParseDuration(val + "ms")`
   - `"min"` → `time.ParseDuration(val + "m")`
2. 更新頂層 Model 的 `valuesList`、`loopEnabled`、`loopCount`、`loopInterval`
3. `vars.LoadValuesFiles(valuesList)` → mergedVals
4. `config.ScanFunctions(globalCfg.FuncsDir)` → functions
5. 合併 values + defaults + functions + builtins，計算每個 key 的 `varSources`
6. 切換到 `ScreenVars`

**預設值初始化**（進入畫面 3 時）：
- `valDirInput` 預填 `globalCfg.ValuesDir`
- 自動掃描 `globalCfg.ValuesDir` 下的 `*.toml`/`*.json` 供使用者選擇加入 `valFileList`
- `loopUnit` 預設 `"s"`

---

### 6.6 `screen_template.go` — 畫面 2：選擇模板

**狀態**：
```go
type templateModel struct {
    items list.Model // 每項為 TemplateFileInfo
}
```

**初始化觸發**（進入畫面 2 時）：
- 呼叫 `template.ScanTemplates(globalCfg.TemplateDir)` → 填入 `items`
- 若目錄不存在 → 顯示錯誤訊息，保持在畫面 2

**按鍵處理**：
| 按鍵 | 動作 |
|------|------|
| `↑`/`↓`/`j`/`k` | 移動游標 |
| `g`/`G` | 跳至頭/尾 |
| `Enter` | 選定模板 → 觸發 `loadTemplateCmd`（非同步讀取 entries）→ 進入畫面 3（值檔設定） |
| `Esc` | 返回畫面 1 |

**`loadTemplateCmd` 副作用**（精簡，Values 在畫面 3 載入）：
- `template.Load(selectedPath)` → 取得 `map[string]domain.TemplateEntry`，存入頂層 Model
- 切換到 `ScreenValues`（畫面 3）

---

### 6.7 `screen_vars.go` — 畫面 4：變數總覽與覆蓋

**狀態**：
```go
type varsModel struct {
    table       table.Model   // bubbles/table：Key | 來源 | 值
    editingKey  string        // 目前編輯中的 key（空字串 = 無編輯）
    editInput   textinput.Model
    smartEdit   *smartEditModel // 僅陣列/物件值展開時非 nil
    expandedKeys map[string]bool // 記錄哪些 key 的 Smart Edit 已展開
}
```

**Table 欄位**：
| 欄位 | 寬度 | 說明 |
|------|------|------|
| Key | 20 | 變數名稱 |
| 來源 | 10 | `cli`/`values`/`defaults`/`func`/`builtin` |
| 值 | 剩餘 | 字串值，或 `[ N 項 ] ▶`（陣列/物件，可展開） |

> **Smart Edit 展開/收起（toggle）**：陣列或物件類型的值在 Table 中顯示為 `[ N 項 ] ▶`（可展開）。首次按 `Enter` 後展開 Smart Edit 面板，同時該 row 狀態改為 `[ N 項 ] ▼`（展開中）；在 Smart Edit 面板內按 `Enter` 確認編輯後，row 更新為 `[ 已覆蓋 N 項 ] ▼`；按 `r` 重設則回到 `[ N 項 ] ▶`。

**來源標籤計算邏輯**：
```
若 key 在 varOverrides → "cli"
否則若 key 在 mergedVals → "values"
否則若 key 在 entry.Defaults → "defaults"
否則若 key 在 functions → "func"
否則若 key 是 builtin → "builtin"
```

**按鍵處理**：
| 按鍵 | 動作 |
|------|------|
| `↑`/`↓`/`j`/`k` | 移動 table 游標 |
| `e` | 對選中的 row 進入 editInput（顯示目前值，可修改） |
| `Enter`（editInput 開啟） | 確認覆蓋，`varOverrides[key] = input.Value()` |
| `Enter`（陣列/物件 row） | 展開 / 收起 Smart Edit（toggle） |
| `r` | 清除 `varOverrides[key]`，回到合併值 |
| `Tab` | 進入畫面 5 |
| `Esc`（editInput 開啟） | 取消編輯 |
| `Esc`（smartEdit 開啟）| 關閉 Smart Edit（不儲存） |
| `Esc`（一般狀態） | 返回畫面 3 |

---

### 6.8 `smart_edit.go` — 結構化 JSON 編輯

Smart Edit 僅適用於值類型為 `[]any` 或 `map[string]any` 的變數。

```go
// smartEditModel 管理結構化 JSON 編輯介面。
type smartEditModel struct {
    key      string       // 編輯中的變數名稱
    rows     []editRow    // 每一行對應陣列中的一個元素
    cursor   int
    done     bool         // 使用者按下 Enter 確認
    cancelled bool
}

// editRow 代表陣列中一個元素的可編輯欄位。
// 針對 map 類型的元素，每個 key 對應一個 textinput。
type editRow struct {
    fields  map[string]textinput.Model
    enabled bool // 勾選狀態（[x] / [ ]）
}

// Result 序列化為 JSON 字串，供 varOverrides 存入。
func (m *smartEditModel) Result() (string, error)
```

**按鍵處理**（Smart Edit 開啟時）：
| 按鍵 | 動作 |
|------|------|
| `↑`/`↓` | 移動 row 游標 |
| `Tab` | 在同一 row 的不同欄位間切換 |
| `Space` | 切換 row 的 enabled 狀態 |
| `a` | 新增空白 row |
| `Enter` | 確認，將結果序列化存入 `varOverrides` |
| `Esc` | 取消，不修改 `varOverrides` |

---

### 6.9 `screen_entry.go` — 畫面 5：選擇 Entry

**狀態**：
```go
type entryModel struct {
    items       list.Model // entry 名稱列表
    cliPreview  string     // 即時合成的 CLI 指令字串
}
```

**CLI 預覽合成邏輯**：
```go
func buildCLIPreview(
    conn string, tmplPath string, entryName string,
    valuesList []string, loopCount int, loopInterval time.Duration,
    overrides map[string]string,
) string {
    parts := []string{
        "nats-runner",
        "-c", conn,
        "-t", tmplPath,
        "-n", entryName,
    }
    for _, v := range valuesList {
        parts = append(parts, "--values", v)
    }
    if loopCount != 1 {
        parts = append(parts, "--loop", strconv.Itoa(loopCount))
    }
    if loopInterval > 0 {
        parts = append(parts, "--interval", loopInterval.String())
    }
    for k, v := range overrides {
        parts = append(parts, fmt.Sprintf("%s=%s", k, v))
    }
    return strings.Join(parts, " ")
}
```

**按鍵處理**：
| 按鍵 | 動作 |
|------|------|
| `↑`/`↓`/`j`/`k` | 移動游標 |
| `g`/`G` | 跳至頭/尾 |
| `Enter` / `F5` | 選定 entry → 進入畫面 6，觸發執行 |
| `Esc` | 返回畫面 4 |

---

### 6.10 `screen_exec.go` — 畫面 6：執行監控

**狀態**：
```go
type execModel struct {
    results     []LoopResult // 已完成的迭代
    current     int          // 目前迭代序號
    total       int          // 0 = 無限
    interval    time.Duration
    paused      bool
    done        bool         // 執行完畢（loop 已跑完或被中斷）
    returnTarget Screen      // Esc 後返回的目標（ScreenValues 或 ScreenEntry）
    logger      *logger.Logger
    logPath     string
    showLogPath bool         // 按 l 後顯示
    cancelFn    context.CancelFunc
}
```

**bubbletea 非同步執行**：
- 執行開始時發送 `startExecCmd`（回傳 `tea.Cmd`）
- 每次迭代完成後透過 `execResultMsg` 通知 Model 更新
- `execResultMsg` 結構：`{ Iteration int; Result LoopResult }`
- 暫停：設定 `paused = true`，在 Loop 中等待 `!paused`（使用 channel 或輪詢）

**精簡狀態列更新**：
- Table 只保留最近 10 筆結果（避免畫面過長）
- 簡短回覆：截取 reply 的前 60 字元 + 省略符號

**按鍵處理**：
| 按鍵 | 動作 |
|------|------|
| `p` | 暫停 / 繼續 Loop |
| `l` | 切換顯示 Log 路徑 |
| `r` | **執行完畢後**：以相同設定重新執行 |
| `Esc` | **執行完畢後**：返回 `returnTarget`（畫面 3 重新選值檔或畫面 5 重新選 Entry）；**執行中** → no-op |
| `Ctrl+C` | 執行中：中斷執行，切換到 `done = true` 狀態 |

> 執行完畢（`done = true`）後，畫面底部顯示提示列：
> `[ Esc ] 返回選單  [ r ] 重新執行  [ q ] 退出`

---

### 6.11 `run.go` — TUI 進入點

```go
// Run 初始化並啟動 TUI。
// 由 cli/root.go 在偵測到無 -t/-n 旗標時呼叫（互動模式）。
func Run(version string) error {
    gc, _ := config.LoadGlobalConfig()
    m := initialModel(gc)
    p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
    _, err := p.Run()
    return err
}
```

**`cli/root.go` 進入點邏輯**：
```go
// 若 -t 與 -n 皆未提供 → 進入 TUI
if *templatePath == "" && *templateName == "" {
    dieIfErr(tui.Run(version))
    return
}
// 否則 → 原有 CLI 邏輯
```

---

## 七、`internal/cli/root.go` 完整更新後執行流程

```
Execute(args, version)
│
├─ "config" subcommand → handleConfigCmd()
│
├─ -t/-n 皆空 → tui.Run(version)   ← TUI 互動模式
│
└─ 一般 CLI 模式：
   ├─ 1. 解析旗標（-c, -t, -n, --values×N, --loop, --interval）
   ├─ 2. ResolveConnection(*c) → connCfg, sourceLabel
   ├─ 3. LoadGlobalConfig() → gc（取 FuncsDir）
   ├─ 4. ScanFunctions(gc.FuncsDir) → functions
   ├─ 5. template.Load(*t) → entries
   ├─ 6. template.GetEntry(entries, *n) → entry
   ├─ 7. LoadValuesFiles(*values) → mergedVals
   ├─ 8. logger.New() → log
   ├─ 9. execLoop(ctx, nc, entry, resolveCtx, *loop, interval, log, printResult)
   └─ 10. log.Close(); fmt.Println("Log:", log.Path())
```

---

## 八、破壞性變更清單

| 項目 | 現有行為 | 新行為 | 影響 |
|------|---------|-------|------|
| `~/.nats-runner.toml` 欄位 | `default_config_path = "/abs/path"` | `default_connection = "primary"` | 使用者需重新 `config set` |
| `vars.Resolve` 簽章 | `(body, cli, defaults, funcs map[string]string)` | `(body string, ctx ResolveContext)` | 所有呼叫端需更新 |
| `config.LoadAppConfig` | 讀取含 `[functions]` 的單一 toml | 廢除，改為 `LoadConnectionFile` + `ScanFunctions` | `cli/root.go` 需更新 |
| `config.ResolveConfigPath` | 回傳路徑字串 | 廢除，改為 `ResolveConnection` | `cli/root.go` 需更新 |
| `config.SaveGlobalConfig(absPath string)` | 只存路徑 | 接受 `*GlobalConfig`，merge 寫入 | `cli/config_cmd.go` 需更新 |
| Template body 語法 | `{{key}}` regex 替換 | `{{ .key }}` Go template | 現有 templates/ 下所有 .toml 的 body 需更新 |
| `AppConfig.Functions` toml tag | `toml:"functions"` | 無 toml tag（runtime-only） | `configs/metering.toml` 的 `[functions]` 需移除 |

---

## 九、檔案結構總覽（完成後）

```
internal/
  cli/
    root.go          — Execute + loop + TUI 分派
    config_cmd.go    — config set/show/list
  config/
    loader.go        — ResolveConnection, LoadConnectionFile,
                       ScanFunctions, ListConnections,
                       LoadGlobalConfig, SaveGlobalConfig
  domain/
    model.go         — AppConfig, ConnectionConfig, GlobalConfig,
                       FuncConfig, TemplateEntry, StreamConfig, TLSConfig
  logger/
    logger.go        — Logger, Entry   [新增]
  nats/
    executor.go      — ExecReq, ExecPub, ExecJS   [不變]
    options.go       — Connect               [不變]
  template/
    loader.go        — Load, GetEntry, ScanTemplates  [新增 ScanTemplates]
    loader_test.go
  tui/               — [全新套件]
    model.go
    run.go
    screen_global.go
    screen_template.go
    screen_values.go   — [新增]
    screen_vars.go
    screen_entry.go
    screen_exec.go
    smart_edit.go
    filepicker.go
    styles.go
    strings.go         — TUI 正體中文字串常數  [新增]
  vars/
    resolver.go      — Resolve, ResolveContext, buildDataMap, buildFuncMap
    values_loader.go — LoadValuesFiles   [新增]
    resolver_test.go

configs/
  primary.toml       — [只含 [connection]，移除 [functions]]
  metering.toml      — [只含 [connection]]

funcs/               — [新增目錄]
  uuid.toml
  now_ms.toml

values/              — [新增目錄，可選]
```

---

## 十、測試策略

### 單元測試（每階段必須通過）

| 套件 | 主要測試目標 |
|------|------------|
| `config/` | `ResolveConnection` 優先順序、`LoadConnectionFile` auth 驗證、`ScanFunctions` 邊界條件、`SaveGlobalConfig` merge 行為 |
| `vars/` | 五層優先順序、`LoadValuesFiles` 多格式與合併、`Resolve` Go template 各指令、`missingkey=error` 觸發 |
| `template/` | `ScanTemplates` 正常/空目錄/格式錯誤 |
| `logger/` | `WriteEntry` goroutine-safe、檔名格式 |
| `cli/` | （整合測試）旗標解析、`config set` 驗證 |

### 整合測試

- CLI 端對端：`--values` 多檔覆蓋 + `key=val` 覆蓋 + template render 結果正確
- Loop：執行 3 次，每次結果獨立，log 有 3 筆 entry

### TUI 測試

- 使用 `bubbletea` 的 `tea.NewProgram` 搭配 `tea.WithInput(...)` 模擬按鍵
- 至少測試：畫面轉換（1→2→3→4→5→6）、`Esc` 返回、`s` 儲存

---

## 十一、開發建議優先順序

```
P0-1: domain/model.go 更新（無邏輯，5 分鐘）
P0-2: config/loader.go 重寫（基礎，其他套件依賴）
P0-3: cli/config_cmd.go 重寫
P0-4: cli/root.go 旗標更新（暫時不呼叫新功能，先讓舊功能通過測試）
P0-5: 更新 configs/metering.toml，建立 funcs/ 目錄範例
      → go test ./... 全通過

P1-1: vars/values_loader.go（獨立，先寫測試）
P1-2: vars/resolver.go 重寫（Go template 引擎）
P1-3: template/loader.go 新增 ScanTemplates
P1-4: cli/root.go 接入新 Resolve 流程
      → go test ./... 全通過

P2-1: logger/logger.go
P2-2: cli/root.go 接入 execLoop + logger
      → go test ./... 全通過

P3-1: go.mod 加入 bubbletea 依賴
P3-2: tui/styles.go + tui/model.go 骨架
P3-3: screen_global.go（精簡為連線選擇）
P3-4: screen_template.go
P3-5: screen_values.go（值檔 + Loop 設定，含單位切換與 filepicker）
P3-6: screen_vars.go + smart_edit.go
P3-7: screen_entry.go（CLI 預覽合成）
P3-8: screen_exec.go（接入 execLoop，含 Esc 返回）
P3-9: strings.go（所有正體中文 UI 字串常數）
P3-10: cli/root.go 進入點分派
      → 手動整合測試（TUI 無法純單元測試）
```
