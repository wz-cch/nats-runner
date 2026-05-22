# Metering 手冊

[[_TOC_]]

---

# 1. 系統概覽

## 1.1 背景

Metering 系統負責採集與查詢各類資源的使用量，並提供兩種觀察視角：

### SRP 視角

Tenant 查詢自身購買的服務包（Service Ready Package）總用量，隱藏底層 Resource 組成。

### 供應商視角

Resource 供應商查詢特定 Resource Type 的跨 Tenant 總體用量。

---

## 1.2 核心概念

| 名詞 | 說明 | 範例 |
|---|---|---|
| **Tenant** | 租戶 | `testtenant001` |
| **ResourceId** | 資源實例 ID，對應 IoTDB 路徑第四段 | `vm-001`、`db-prod-01` |
| **ResourceType** | 底層資源類型，由供應商定義，對 Tenant 透明 | `compute`、`api`、`db` |
| **SRPType** | 服務包樣版，由多個 ResourceType 組成 | `ai-service-package` |
| **Metric** | 具體量測指標 | `vcpu_count`、`calls`、`aitoken` |

---

# 2. 通訊架構

## 2.1 API 總覽

| 功能類型 | NATS Subject |
|---|---|
| ResourceType 管理 | `eco1j.infra.metering.resource-types.{action}` |
| SRPType 管理 | `eco1j.infra.metering.srp-types.{action}` |
| SRP 用量查詢 | `eco1j.infra.metering.usage-stats.srp` |
| Resource 用量查詢 | `eco1j.infra.metering.usage-stats.resource` |

---

## 2.2 NATS 訊息格式

### Request

```json
{
  "reqSeqId": "req-001",
  "timestamp": 1768363186000,
  "data": {}
}
```

### Reply（成功）

```json
{
  "reqSeqId": "req-001",
  "status": "ok",
  "data": {}
}
```

### Reply（失敗）

```json
{
  "reqSeqId": "req-001",
  "status": "error",
  "error": "error_code",
  "message": "說明文字"
}
```

---

# 3. ResourceType API

## 3.1 建立 ResourceType

### Subject

```text
eco1j.infra.metering.resource-types.create
```

### 錯誤碼

| Error Code | 說明 |
|---|---|
| `resource_type_already_exists` | ResourceType 已存在 |

### Request 範例

```json
{
  "reqSeqId": "req-001",
  "data": {
    "resourceType": "api",
    "description": "API 呼叫資源",
    "metrics": [
      {
        "field": "calls",
        "type": "sum"
      }
    ]
  }
}
```

---

## 3.2 查詢 ResourceType

### Subjects

| 功能 | Subject |
|---|---|
| 查詢單一 | `eco1j.infra.metering.resource-types.get` |
| 查詢全部 | `eco1j.infra.metering.resource-types.list` |

### 錯誤碼

| Error Code | 說明 |
|---|---|
| `resource_type_not_found` | ResourceType 不存在 |

---

## 3.3 更新 ResourceType

### 規則

- `resourceType` 為必要欄位。

---

## 3.4 刪除 ResourceType

### 規則

若有 SRPType 正在使用該 ResourceType，需拒絕刪除。

### 錯誤碼

| Error Code | 說明 |
|---|---|
| `resource_type_in_use` | ResourceType 已被 SRPType 引用 |

---

# 4. SRPType API

## 4.1 建立 SRPType

### Subject

```text
eco1j.infra.metering.srp-types.create
```

### 規則

- `resources` 中的所有 `ResourceType` 必須預先存在。

### Request 範例

```json
{
  "reqSeqId": "req-010",
  "data": {
    "srpType": "ai-service-package",
    "description": "AI 服務包",
    "resources": [
      "api",
      "compute"
    ],
    "metrics": [
      {
        "field": "aitoken",
        "type": "sum"
      }
    ]
  }
}
```

---

# 5. 用量查詢（Usage Stats）

## 5.1 查詢參數

| 參數 | 必填 | 說明 |
|---|---|---|
| `startTime` | ✅ | ISO 8601 或 Epoch 毫秒 |
| `endTime` | ✅ | ISO 8601 或 Epoch 毫秒 |
| `tenantId` | ❌ | 指定租戶；未帶入時回傳全部租戶聚合資料 |
| `skipCount` | ❌ | 分頁跳過筆數，預設 `0` |
| `maxResultCount` | ❌ | 每頁最大筆數（1 ~ 100），預設 `20` |

---

## 5.2 SRP 用量查詢

### Subject

```text
eco1j.infra.metering.usage-stats.srp
```

### Reply 成功範例

```json
{
  "status": "ok",
  "data": {
    "srpType": "ai-service-package",
    "items": [
      {
        "tenantId": "testtenant001",
        "resources": [
          {
            "type": "api",
            "metrics": [
              {
                "name": "calls",
                "usage": 83000
              }
            ]
          },
          {
            "type": "ai-service-package",
            "metrics": [
              {
                "name": "aitoken",
                "usage": 2180
              }
            ]
          }
        ]
      }
    ],
    "totalCount": 1,
    "hasMore": false
  }
}
```

---

# 6. 驗證規則

| 類別 | 規則 |
|---|---|
| 唯一性 | `resourceType` 與 `srpType` 名稱不可重複 |
| 關聯性 | 建立 SRPType 時，底層 `resources` 必須已存在 |
| 依賴保護 | 刪除 ResourceType 前需檢查是否被 SRPType 引用 |
| 查詢安全 | `startTime` 與 `endTime` 為必填，禁止無時間範圍查詢 |
| 時間有效性 | `endTime` 必須大於 `startTime` |