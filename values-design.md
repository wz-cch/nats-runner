# nats-runner 值檔 (Values) 與模板設計

> 狀態：定案  
> 日期：2026-05-22

## 一、目的

消除模板重複。當前不同業務（如 `iot_suite`、`tb_package`）結構重複率 >70%，差異僅在少數欄位值。透過**共用骨架模板**與**值檔 (Values)** 分離，減少維護成本。

---

## 二、Body 渲染採用 Go text/template

模板文件中的 `body` 區塊改用**反引號 (```)** 包裹，內容支援完整的 **Go text/template** 語法。

### 支援指令

| 指令 | 用法 |
|--|---|--|
| `if` / `else` / `end` | 條件分支 |
| `range` / `end` | 陣列/對像迴圈 |
| `with` / `end` | 作用域改變 |
| `{{ .Key }}` | 變數讀取 |
| `{{ .Data | pipe }}` | 管道運算 (`toJson`, `trim`) |

### 範例 (template/base.srp.toml)

```toml
[srp_create]
subject  = "eco1j.infra.metering.srp-types.create"
mode     = "req"
defaults = { srp_type = "iotsuite", description = "IoT Suite package" }

body = `
{
  "reqSeqId":   "{{ uuid }}",
  "timestamp":  {{ now_ms }},
  "data": {
    "srpType":    "{{ .srp_type }}",
    "description":"{{ .description }}",
    "resources":  {{- if .resources }}{{ .resources | toJson }},{{- else }}[]{{- end }}
    "metrics":    {{ .metrics | toJson }}
  }
}
`
```

> `defaults` 為**純靜態字串**，不經 Go template 渲染。若需依業務差異化，請改用 values 檔覆蓋。

### 內建 Pipe Functions

| 函數 | 說明 | 範例 |
|------|------|------|
| `toJson` | 將 Go 值序列化為 JSON 字串 | `{{ .metrics \| toJson }}` |
| `trim` | 去除字串頭尾空白 | `{{ .desc \| trim }}` |

> Pipe functions 在 `template/` 套件的 `FuncMap` 中統一註冊。

---

## 三、多檔合併機制 (Helm 風格)

支援多份值檔合併，優先權由低到高覆蓋。使用者可依業務需求調整選擇順序。

### 優先級順序（由低至高）

| 優先級 | 來源 | 說明 |
|--------|------|------|
| 1（最低） | 內建變數 | `uuid`、`now`、`now_ms`、`now_iso` |
| 2 | `funcs/*.toml` Shell 函數 | 執行 shell 指令取得結果 |
| 3 | 模板 `defaults` | 模板 entry 中的靜態預設值 |
| 4 | `--values` 值檔（可多份） | 依指定順序合併，越晚指定優先權越高 |
| 5（最高） | CLI `key=val` | 使用者手動覆寫 |

### 值檔 (Values) 格式

支援 **TOML** 與 **JSON** 兩種格式，方便 Python 或其他程式語言直接生成：

```toml
# values/iot.toml
srp_type      = "iotsuite"
description   = "IoT Suite package"
resources     = []
metrics = [
  { field = "tag", type = "sum" },
  { field = "dashboard", type = "sum" }
]
```

### 變數解析最終順序

1. 內建變數（`uuid`、`now`、`now_ms`、`now_iso`）—— 最低優先
2. `funcs/*.toml` Shell 函數結果（cached per key per run）
3. 模板 `defaults`（純靜態字串）
4. `--values` 合併後的值檔（越晚指定優先權越高）
5. CLI `key=val` —— 最高優先

---

## 四、與現有設計關聯

- **全面升級**：`body` 統一使用 Go `text/template` 引擎渲染；舊的 `{{key}}` 雙括號語法不再支援。
- **值檔覆蓋**：值檔內的值優先覆蓋模板中的 `defaults`。
- **多格式支持**：`--values` 自動偵測 `.toml` 或 `.json` 解析。
- **Builtin 語法**：內建變數（`uuid`、`now_ms` 等）以 template function 形式呼叫（`{{ uuid }}`，無 `.`）；資料變數以 `.key` 形式讀取（`{{ .srp_type }}`）。

---

## 五、使用範例

透過指定值檔，自動將模板渲染為該業務的專屬格式：

```bash
# 單一業務值檔
./nats-runner -c primary --values values/iot.toml \
  -t templates/base.srp.toml -n srp_create

# 多檔層疊 (extra 覆蓋 base)
./nats-runner -c primary --values values/iot.toml --values values/extra_iot.toml \
  -t templates/base.srp.toml -n srp_create
```
