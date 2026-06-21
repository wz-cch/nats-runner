# nats-runner 問題清單與重構計畫

> 建立日期:2026-06-15
> 來源:對 `design.md` 等設計文件與 `internal/` 程式碼的完整 review(已驗證 `go build` / `go vet` / `go test ./...` 通過,GOWORK=off)。
> 用途:作為本次重構的問題追蹤清單。每項含「位置 / 描述 / 影響 / 修正方向 / 狀態」。

---

## 摘要

| 分類 | 嚴重度分布 | 重點 |
| :--- | :--- | :--- |
| TUI 子系統 | 2 高 / 5 中 / 7 低 | 流程順序倒置、**無法輸入必填參數**、多個功能實際是死碼、停止失效 |
| 核心(非 TUI) | 1 高 / 1 中 / 3 低 | **變數替換無 JSON 跳脫**、functions eager 執行、I/O 與邏輯混雜 |
| 文件漂移 | — | Cobra/快取語意/舊語法/config 用法/優先序皆與實作不符 |

**整體判斷**:核心 CLI 路徑(non-TUI)分層乾淨、可測、錯誤處理嚴謹;問題集中在「TUI 子系統尚未接線完成且 UX 流程倒置」與「文件落後於 text/template 重構」。無結構性重寫需求,但 TUI 需重新設計。

狀態圖例:`[ ]` 待處理 · `[~]` 進行中 · `[x]` 已修正

> ✅ **2026-06-15 重構已全數修正以下項目**（TUI 改為雙欄即時預覽設計 + 核心 C1–C5 + 文件 D1–D6）。
> 驗證:`GOWORK=off go build ./...`、`go vet ./...`、`go test ./...` 全數通過(含新增的 vars / tui / templates 測試)。

---

## A. TUI 子系統(本次重新設計)

| ID | 嚴重度 | 位置 | 問題 | 影響 | 修正方向 | 狀態 |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| **T1** | 高 | `internal/tui/model.go` `Run()`(~150);讀於 `model.go:349`、`screen_vars.go:109/112` | `m.funcs` 從未被賦值(`ScanFunctions` 只在 CLI 路徑呼叫) | TUI 模式下 shell functions 完全不執行,函數列永遠空白 | `Run()` 載入 `gc` 後呼叫 `config.ScanFunctions(gc.FuncsDir)` 填入 model | [x] |
| **T2** | 高 | `internal/tui/model.go:345-350`;`smart_edit.go:40` | `startExecCmd` 組 `ResolveContext` 未設 `CLIParams`,`varRows` 的編輯被丟棄 | 整個變數編輯/覆寫 UI 對執行**毫無作用** | 將使用者輸入的變數收成 map 餵入 `ResolveContext.CLIParams` | [x] |
| **T3** | 高(UX 致命傷) | `internal/tui/screen_vars.go:103-148` | `buildVarRows` 只列 `defaults`/funcs/builtins,**從不掃描 body 的 `{{ .var }}`**;`break` 只看第一個 entry;map 順序不定 | 模板真正需要的必填參數(如 `srp_type`、`description`)根本不會出現,**TUI 無法送出參數化請求** | 改為:選定 entry 後掃描 body 取得所有 `{{ .x }}`,逐欄列出並標示來源/待輸入 | [x] |
| **T4** | 高 | `internal/tui/screen_exec.go:73-79`;goroutine 在 `model.go:355` | ctrl+c 只設旗標;goroutine 僅靠 `signal.NotifyContext` 取消,但 bubbletea alt-screen 攔截 ctrl+c 成 KeyMsg,OS 收不到 SIGINT | 「停止」無效:背景持續對 NATS publish,且阻塞於 channel send 造成 goroutine 洩漏 | 將 `context.CancelFunc` 存入 model,ctrl+c 時呼叫;`Update` append 前以 `execDone` 守門 | [x] |
| **T5** | 中 | `internal/nats/executor.go:53,80`(被 `model.go:399,401` 呼叫) | `ExecPub`/`ExecJS` 直接 `fmt.Printf` 到 stdout | 在 TUI alt-screen 下破壞畫面;pub/js 結果未走 TUI | 改為回傳字串(比照 `ExecReqReply`),由呼叫端決定輸出(見 C3) | [x] |
| **T6** | 中 | `internal/tui/model.go:435`;顯示於 `screen_exec.go:59-61,86-89` | goroutine 算了 `log.Path()` 卻丟棄,`m.execLogPath` 永不設定 | TUI 永遠不顯示 log 路徑;對應顯示區與 `l` 鍵是死碼 | 透過 `execPayload`/`execResultMsg` 把路徑帶回 model 並顯示 | [x] |
| **T7** | 中 | `internal/tui/helpers.go`(`templateEntryNamesFromMap`) | 註解稱 sorted 但未排序(map 遍歷) | entry 列順序與 `entryIdx` 選取每次 run 都可能變 | 回傳前 `sort.Strings` | [x] |
| **T8** | 中(UX) | 流程設計(`model.go` Screen 列舉) | 流程順序倒置:Template → Values → Vars → Entry;在知道 entry/body 前就設 values、看變數;values/loop 在畫面 1 與 3 重複 | 操作不直覺、狀態重複 | 重排為:連線 → 模板/Entry → 填變數 → 選項+預覽 → 執行(見下方重設計) | [x] |
| **T9** | 低 | `internal/tui/screen_values.go:124` | 新增 values 檔硬寫 `"values/new.toml"` placeholder | `a` 鍵等同 stub,會加入不存在的檔 | 提供路徑輸入,或從 `values_dir` 選檔 | [x] |
| **T10** | 低 | `internal/tui/screen_values.go`(commit loop input) | loop count 可被設為 `0` 且 loop 啟用 → 無 interval 的無限緊迴圈 | 無驗證,易誤觸 | commit 時 clamp count ≥ 1;明確處理 interval 下限 | [x] |
| **T11** | 低 | `internal/tui/smart_edit.go:53-54` | rune 輸入直接 append `msg.String()`,多字元(貼上)會含 `"runes:"` 前綴 | 貼上會污染編輯值 | 改用 `string(msg.Runes)` | [x] |
| **T12** | 低 | `internal/tui/screen_exec.go:44` vs `model.go:408` | `StatusOK = "OK!"` 與實際寫入的 `"OK"` 不一致,靠 `HasPrefix(...,"OK")` 巧合運作 | 若錯誤訊息以 "OK" 開頭會被誤染綠 | 統一用常數比對 | [x] |
| **T13** | 低 | `internal/tui/screen_exec.go:21` vs `model.go:287` | `viewExec` 的 Cmd 預覽缺 `-t`/`--values`/`--loop`,與 `buildCLIPreview` 不一致 | 顯示兩種不同的「等效指令」 | 共用 `buildCLIPreview` | [x] |
| **T14** | 低 | `internal/tui/strings.go`;`screen_vars.go:88-93` | 多個未使用常數;`editActive` 欄位與 `e` 鍵設了狀態卻無人讀(no-op) | 死碼 | 移除或接線 | [x] |

---

## B. 核心(非 TUI)

| ID | 嚴重度 | 位置 | 問題 | 影響 | 修正方向 | 狀態 |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| **C1** | 高 | `internal/vars/resolver.go`(`Resolve`) | 用 `text/template`(非 `html/template`)直接將字串插入 JSON body,**無跳脫** | 值含 `"`、`\`、換行(如 `description='AI "Pro"'`)→ 產生非法 JSON payload | 對字串欄位建立 JSON-safe 慣例:提供預設跳脫 helper 或在文件/模板統一用 `{{ .x | toJson }}`;評估字串自動跳脫 | [x] |
| **C2** | 中 | `internal/vars/resolver.go:78`(`buildDataMap`) | 遍歷 `ctx.Functions` 全部執行,不論 body 是否引用 | `--loop` 下每次 iteration 為每個 function spawn 子行程,浪費且可能有副作用 | 只執行 body 實際引用到的函數(掃描 template node 或 lazy 求值) | [x] |
| **C3** | 低 | `internal/nats/executor.go:49-55,64-82` | `ExecPub`/`ExecJS` 把輸出 I/O 與執行邏輯混在一起 | 無法在 TUI 重用;與 T5 同源 | 統一回傳結果字串,輸出交呼叫端;CLI 端負責印 stdout | [x] |
| **C4** | 低 | `internal/domain/model.go:10`(`AppConfig.Functions`) | 欄位從未被填入(root.go:95、model.go:358 只設 Connection) | 死欄位,易誤導 | 移除,或實際改用此欄位傳遞 functions | [x] |
| **C5** | 低 | `internal/cli/root.go:70,73` | `ResolveConnection` 內已 `LoadGlobalConfig`,root 又呼叫一次 | 輕微重複 | 合併讀取 | [x] |

---

## C. 文件漂移(程式碼為準需更新)

> 準確可作為改寫依據的文件:`values-design.md`、`config-design.md`、`interactive-design.md`、`dev-spec.md`。
> 最需修正:`AGENTS.md`、`README.md`(中英兩半)、`design.md`。

| ID | 出處 | 錯誤敘述 | 應為 | 狀態 |
| :-- | :-- | :-- | :-- | :-- |
| **D1** | `AGENTS.md:27`、`README.md:491` | CLI 使用 **Cobra** | 標準 `flag` 套件 | [x] |
| **D2** | `AGENTS.md:75`、`README.md:161/398`、`design.md §4.3` | 「`{{uuid}}` 出現兩次 = 同值(快取)」 | 內建 `{{ uuid }}` **每次出現都產生新值**;只有 shell function 以 `{{ .name }}` 引用時才單值(`dev-spec.md:375` 為正) | [x] |
| **D3** | `AGENTS/README/design` 範例遍布 | 舊 `{{var}}`/`{{id}}` 無點無空白語法 | 資料變數 `{{ .var }}`、內建 `{{ uuid }}`;body 用反引號 | [x] |
| **D4** | `README.md:88/321`、`design.md §5/§6.2` | `config set <path>`(路徑式) | `config set <name>`(名稱式,解析 `configs/<name>.toml`);另補 `config list`、`config set --*-dir` | [x] |
| **D5** | `AGENTS.md:59-63`、`README.md:147`、`design.md §4.1` | 優先序寫成 4 層 | 5 層:**CLI > --values > defaults > functions > builtins** | [x] |
| **D6** | `design.md §4.2` | 內建清單漏列 `uuid` | `uuid` 為內建函數 | [x] |

---

## D. TUI 重新設計(草案,待確認)

### 現況問題(對應 T3 / T8)
現行 6 畫面:`全域設定 → 模板檔 → Values/Loop → 變數總覽 → Entry → 執行`。核心缺陷:
1. 在選定 entry(因而知道 body)**之前**就要設 values、看「變數總覽」,導致變數總覽只能猜、且永遠列不全。
2. **沒有任何輸入必填參數的途徑** → TUI 實際上無法送出參數化請求。
3. values/loop 設定在畫面 1 與 3 重複出現。

### 建議新流程(精靈式,順序符合資料相依)
```
1. 連線        選 configs/*.toml(顯示 URL / auth_mode)
2. 模板 + Entry 選檔 → 選 entry(此時取得 subject / mode / body)
3. 填變數      掃描 body 的 {{ .x }};逐欄列出,標示來源
                (CLI輸入 / values / defaults / func / builtin / ★待輸入)
                可逐欄輸入值,右側即時顯示 render 後 JSON payload
4. 選項 + 預覽  loop 次數 / interval、values 檔;顯示等效 CLI 指令
5. 執行        串流結果、可即時停止、結束顯示 log 路徑
```
此重設計同時修正 T1–T7(填變數步驟接上 `CLIParams`、掃 body、排序、停止、log 路徑、輸出不破畫面)。

### 進入點(2026-06-15 補充決議)
TUI 改為**明確啟動**:`nats-runner tui` 或 `-i`。裸 `nats-runner`(未帶 `-t`/`-n`)一律印 usage 並 `exit 2`,**不再自動開 TUI** — 維持工具可直接以 CLI / Python subprocess / 自動化方式呼叫,避免在非 TTY 環境出現 `could not open a new TTY`。

### 編輯手感(2026-06-15 補充)
- 變數列為**就地編輯**:聚焦該列直接打字即可,無需「點進去」子畫面。
- 連線/模板/Entry 三列支援 **←/→ 就地切換**(免開選單);清單長時仍可 `Enter` 開完整選單。
- 選擇模板時**自動選取第一個 Entry**,變數列即刻出現,省去額外一步。

---

## E. 建議處理順序
1. **C1**(JSON 跳脫)— 影響所有模式、最隱蔽。
2. **TUI 重新設計**(含 T1–T7)— 讓 TUI 真正可用。
3. **C2 / C3** 與 TUI 低嚴重度收尾(T9–T14)。
4. **文件同步**(D1–D6)— 以 AGENTS.md / README 為先(使用者與 agent 入口)。
