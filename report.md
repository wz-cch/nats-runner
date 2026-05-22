Usage Reporting (JetStream)
------------------------------

此功能用於上報資源使用量數據，透過 **NATS JetStream** 運作，確保數據的持久化（Persistence）與傳輸的可靠性。


### 1. 上報介面定義

*   **Subject**: `eco1j.infra.metering.usage.create` 
    
*   **模式**: Fire-and-Forget (非同步傳輸)
    

#### 請求範例 (Request Example)

JSON

    {
      "reqSeqId": "req-usage-batch-001",
      "timestamp": 1737187200000,
      "data": {
        "usages": [
          {
            "tenantId": "tenant-A",
            "resourceType": "api",
            "resourceId": "/api/v1",
            "metrics": {
              "api-call": 1
            },
            "metadata": {
              "path": "/api/v1",
              "method": "GET"
            },
            "timestamp": 1737187200000
          }
        ]
      }
    }

* * *

#### 請求範例 (Request Example)

```JSON
{
  "reqSeqId": "req-usage-batch-001",
  "timestamp": 1737187200000,
  "data": {
    "usages": [
      {
        "tenantId": "tenant-A",
        "resourceType": "WEDA",   // srp名稱
        "resourceId": "mui",      
        "metrics": {
          "devices": 1,
          "datapoints": 100
        }
        "timestamp": 1737187200000
      }
    ]
  }
}
```
* * *

### 2. 欄位詳細說明

#### A. Data 物件 (Payload)

| **欄位** | **類型** | **必填** | **說明** |
| --- | --- | --- | --- |
| `usages` | `array` | 是 | 使用量記錄列表 (BatchCreateUsageRequest) |

#### B. Usage Record 內容

| **欄位** | **類型** | **必填** | **說明** |
| --- | --- | --- | --- |
| `tenantId` | `string` | 是 | 租戶 ID (用於計費與數據隔離) |
| `resourceType` | `string` | 是 | 資源類型 (量測用量的服務) |
| `resourceId` | `string` | 是 | 資源唯一標識符 (SRP Name) |
| `timestamp` | `int64` | 是 | 數據發生時間 (Unix milliseconds) |
| `metrics` | `object` | 是 | 關鍵指標數據 (Key-Value，Value 須為**數值**) |
| `metadata` | `object` | 否 | 額外標籤或描述資訊 (Key-Value，Value 為**字串**) |

* * *

### 3. 回應與機制 (Response)

本介面採用 **非同步機制**：
*   **無應用層回覆**: 格式正確且寫入 Stream 後，系統不會回傳應用層級的 Response。
    
*   **ACK 機制**: 雖然是 Fire-and-Forget，但建議發送端應確認 NATS Server 的 **PubAck**，以確保訊息已成功持久化於 JetStream 中。
    
*   **錯誤處理**: 若發生重大錯誤且系統配置了回覆 Subject，系統才會嘗試進行錯誤通知。
    
