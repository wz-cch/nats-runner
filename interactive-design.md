# nats-runner 互動模式 (`run`) 設計規格

> 狀態：定案  
> 日期：2026-05-22

---

## 一、核心設計理念

將 `nats-runner` 打造為**工作平台**，利用 **bubbletea TUI** 架構提供接近 **k9s** 的操作體驗。

1.  **減少配置成本**：自動讀取本地連線 (`configs/`) 與模板 (`templates/`)，不需每次手動輸入。
2.  **結構化編輯 (Smart Edit)**：自動處理 JSON 跳脫字，使用者只需在 TUI 表格中填入數值。
3.  **視覺化重複發送**：內建 Loop 模式，適合即時監控與 Ping 測試。
4.  **Log 檔案輸出**：完整 Request/Reply 皆寫入檔案，避免長時間追蹤時螢幕資訊被覆蓋。

---

## 二、核心流程

TUI 分為**持久設定**與**執行流程**兩部分：

- **持久設定**（畫面 1）：連線環境、Loop 參數、Values 目錄，修改後寫回 `~/.nats-runner.toml`，下次啟動自動載入。
- **執行流程**（畫面 2–5）：每次調用依序走過。**選定模板後即可得知所有變數**，可預覽並手動覆蓋，再選 Entry 執行。

```text
  [持久設定]
  ┌───────────┐
  │ 連線環境  │
  │ Loop 設定 │──→  選模板  →  變數總覽/覆蓋  →  選 Entry  →  執行監控
  │ Values 目錄│     (畫面 2)      (畫面 3)         (畫面 4)    (畫面 5)
  └───────────┘
   (畫面 1)              ←─────────── Esc 逐步返回 ──────────────────←
```

---

## 三、畫面與細節設計

### 畫面 1：全域設定 (Global Settings)
持久設定介面，連線環境、Values 來源、Loop 參數皆在此設定並儲存。

```
  ┌─── 全域設定 (Global Settings) ──────────────────────────────────┐
  ├── 連線環境 (Connection):                                          │
  │   > primary  (nats://dev-server:4222)                            │
  │     prod     (nats://prod-server:4222)                           │
  │                                                                    │
  ├── Values 來源 (Values Sources):                                   │
  │   目錄: [ ./values/__________ ]  (Enter 開啟檔案瀏覽器自訂路徑)  │
  │   優先順序 (下方優先權較高):                                      │
  │   1. values/iot.toml                                              │
  │   2. values/extra.toml                                            │
  │   [ a 新增 ]  [ d 移除 ]  [ +/- 調整順序 ]                       │
  │                                                                    │
  ├── 迴圈 (Loop):                                                     │
  │   [OFF] [ON]  |  次數: [ 10 ]  |  間隔: [ 60 ]s                   │
  │                                                                    │
  │ [ s 儲存設定 ]                          [ Enter 選擇模板 ──▶ ]   │
  └──────────────────────────────────────────────────────────────────┘
```

> Values 目錄路徑從 `GlobalConfig.values_dir` 讀取；可按 `Enter` 在輸入欄呼叫內建檔案瀏覽器選取自訂路徑。

### 畫面 2：選擇模板 (Select Template)
掃描 `template_dir` 列出所有模板檔案，選定後自動讀取該檔中所有 entry 的變數進入畫面 3。

```
  ┌─── 選擇模板 (Select Template) ──────────────────────────────────┐
  │ 模板目錄: ./templates/                                            │
  │                                                                    │
  │   > base.srp.toml          (SRP CRUD)          5 entries         │
  │     iot_suite.toml         (IoT Suite)         3 entries         │
  │     resource_types.toml    (Resource Types)    4 entries         │
  │     tb_package.toml        (TB Package)        3 entries         │
  │     usage_report.toml      (Usage Report)      2 entries         │
  │                                                                    │
  │ [ Esc 返回全域設定 ]                   [ Enter 選擇模板 ──▶ ]   │
  └──────────────────────────────────────────────────────────────────┘
```

### 畫面 3：變數總覽與覆蓋 (Variables)
選定模板後，TUI 自動合併 values 檔、`defaults`、Shell 函數與內建變數，以表格呈現每個變數的**來源**與**目前值**。使用者可在此對任意變數進行手動覆蓋。

#### 1. 變數總覽 (Overview)
```
  ┌─── 變數總覽 (Variables) ─ [ base.srp.toml ] ───────────────────┐
  │ Key           │ 來源     │ 值                                    │
  │ ─────────────────────────────────────────────────────────────── │
  │ > srp_type    │ values   │ iotsuite                              │
  │   description │ defaults │ IoT Package                           │
  │   metrics     │ values   │ [ 2 項 ]         (Enter 展開編輯)     │
  │   uuid        │ builtin  │ (run-time)                            │
  │   now_ms      │ builtin  │ (run-time)                            │
  │   random_user │ func     │ (shell: shuf -n 1 ./users.txt)        │
  │                                                                    │
  │ [ e 編輯/覆蓋 ]   [ r 重設為合併值 ]   [ Enter 展開 Smart Edit ] │
  │ [ Esc 返回選模板 ]                    [ Tab 進入 Entry 選擇 ──▶ ]│
  └──────────────────────────────────────────────────────────────────┘
```

**來源標籤說明**：
| 標籤 | 代表來源 |
|------|---------|
| `cli` | 本次手動覆蓋（最高優先） |
| `values` | `--values` 值檔中的值 |
| `defaults` | 模板 entry 的 `defaults` |
| `func` | `funcs/*.toml` 的 shell 指令結果 |
| `builtin` | 內建變數（`uuid`、`now`、`now_ms`、`now_iso`） |

#### 2. Smart Edit（結構化編輯）
對 JSON 陣列 / 物件類型的值，按 `Enter` 展開多行結構化編輯，自動處理跳脫字。

```
  ┌── 編輯 [ metrics ] (陣列) ─────────────────────────────────────┐
  │ [x]  field: [ tag _________ ]   type: [ sum _______ ]          │
  │ [x]  field: [ dashboard ___ ]   type: [ sum _______ ]          │
  │ [ ]  field: [ ___________ ]     type: [ ___________ ]          │
  │                                                                  │
  │ [ a 新增列 ]         [ 確認 Enter ]         [ 取消 Esc ]        │
  └────────────────────────────────────────────────────────────────┘
```

### 畫面 4：選擇 Entry
選定要執行的模板條目。底部即時顯示對應的完整 CLI 指令，方便複製到腳本中使用。

```
  ┌─── 選擇 Entry ─ [ base.srp.toml ] ─────────────────────────────┐
  │                                                                   │
  │   > srp_create    創建 SRP Type   (req)                         │
  │     srp_get       查詢 SRP Type   (req)                         │
  │     srp_list      列出全部        (req)                         │
  │     srp_delete    刪除 SRP Type   (req)                         │
  │                                                                   │
  │ CLI: nats-runner -c primary -t base.srp.toml -n srp_create \    │
  │      --values values/iot.toml --loop 10 --interval 60s          │
  │                                                                   │
  │ [ Esc 返回變數 ]                   [ Enter / F5 開始執行 ──▶ ]  │
  └─────────────────────────────────────────────────────────────────┘
```

### 畫面 5：執行監控
為了避免 JSON 內容覆蓋畫面，螢幕僅顯示狀態，詳細內容寫入 Log 檔。

#### A. 精簡狀態列
```
  ┌─── 執行進度 ───────────────────────────────────────────────────┐
  │ Cmd: nats-runner -c primary -n srp_create                       │
  │ Loop: 8 / 10    Interval: 60s                                   │
  │                                                                  │
  │ [ 狀態 ]   [ 耗時 (ms) ]   [ 簡短回覆 ]                        │
  │ [ OK! ]    [    12.5   ]   { "id": "req-001" }                  │
  │                                                                  │
  │ [ p 暫停 ]   [ l 顯示 Log 路徑 ]   [ Ctrl+C 停止 ]             │
  └────────────────────────────────────────────────────────────────┘
```

#### B. Log 輸出 (檔案追蹤)
完整請求與回覆寫入 `logs/nats-runner-YYYYMMDD_HHMMSS.log`。

```
# 檔名：logs/nats-runner-20260522_160000.log
[TIMESTAMP] 2026-05-22 16:00:01
[ACTION]    [REQ] nats://primary/eco1j...
[VALUES]    srp_type: "iotsuite"
[REPLY]     { "status": "ok" ... }
```

---

## 四、UI 架構規劃

1. **框架**：`github.com/charmbracelet/bubbletea`
2. **狀態管理**：`model` 記錄目前所處畫面（`screenGlobal` → `screenTemplate` → `screenVars` → `screenEntry` → `screenExec`），`Esc` 回上一畫面。
3. **Values 順序**：`list` 元件實作 `+/-` 快速移動；`a` 呼叫內建檔案瀏覽器。
4. **Smart Edit**：`table` 或 `textarea` 元件實現結構化 JSON 展開。
5. **CLI 預覽**：Entry 選擇畫面底部即時合成完整的 `nats-runner` 指令。
6. **持久設定**：畫面 1 儲存後寫入 `~/.nats-runner.toml`；每次啟動自動讀取。

---

## 五、CLI 對應旗標

TUI 中所有可設定的參數皆有對應的 CLI 旗標，方便在腳本中直接呼叫，無需啟動 TUI。

```bash
# 等同於在 TUI 中完成全部設定後按下執行
./nats-runner -c primary -t templates/base.srp.toml -n srp_create \
  --values values/iot.toml \
  --loop 10 --interval 60s \
  srp_type=custom_override
```

| 旗標 | TUI 對應位置 | 說明 |
|------|------------|------|
| `--loop <n>` | 全域設定 → Loop 次數 | `0` 表示無限執行 |
| `--interval <duration>` | 全域設定 → Loop 間隔 | 支援 `s`、`ms`、`m`（如 `60s`、`500ms`）|
| `--values <file>` | 全域設定 → Values 順序 | 可重複指定多次，越晚指定優先權越高 |
| `key=val` | 變數總覽 → 手動覆蓋 | 直接覆蓋任意變數（最高優先） |

---

## 六、快捷鍵對照表

| 按鍵 | 適用畫面 | 動作 |
|------|---------|------|
| `↑` / `k` | 所有列表 | 游標上移 |
| `↓` / `j` | 所有列表 | 游標下移 |
| `g` | 所有列表 | 跳至第一項 |
| `G` | 所有列表 | 跳至最後一項 |
| `Enter` | 列表 / 輸入欄 | 確認選擇 / 儲存編輯 |
| `Esc` | 所有（非輸入中） | 返回上一層 |
| `?` | 所有 | 顯示快捷鍵說明 |
| `q` | 所有（非輸入中） | 退出 TUI |
| `Tab` | 畫面 1、3 | 切換面板區塊 |
| `s` | 畫面 1 | 儲存全域設定 |
| `a` | 畫面 1 Values 清單 | 新增值檔（開啟檔案瀏覽器） |
| `d` | 畫面 1 Values 清單 | 移除選中值檔 |
| `+` / `K` | 畫面 1 Values 清單 | 提高選中值檔優先權（上移） |
| `-` / `J` | 畫面 1 Values 清單 | 降低選中值檔優先權（下移） |
| `e` | 畫面 3 變數總覽 | 編輯 / 覆蓋選中變數 |
| `r` | 畫面 3 變數總覽 | 重設選中變數（清除覆蓋，回到合併值） |
| `p` | 畫面 5 執行中 | 暫停 / 繼續 Loop |
| `l` | 畫面 5 | 顯示 Log 檔路徑 |
| `F5` | 畫面 4 | 開始執行（同 Enter） |
| `Ctrl+C` | 畫面 5 執行中 | 中斷執行並退出 |

---

## 七、設計總結

| 使用者需求 | 互動模式 (`run`) 解決方案 |
|---|--|
| 減少配置 | 自動列出 `configs/` 與 `templates/`；連線/Loop 設定持久儲存 |
| 值檔複雜 | 可新增/移除/排序，支援 `values_dir` 與自訂路徑（檔案瀏覽器） |
| 變數透明 | 每個變數顯示來源（cli/values/defaults/func/builtin） |
| 追錯/Log | 螢幕精簡顯示，全量 Log 輸出至 `logs/` |
| 重複發送 | Loop 整合至全域設定，支援次數 + 間隔（秒/分/毫秒） |
| 腳本整合 | TUI 即時生成等效 CLI 指令；所有 TUI 功能皆有對應 CLI 旗標 |
