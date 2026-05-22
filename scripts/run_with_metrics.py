#!/usr/bin/env python3
"""
簡易腳本：所有參數直接內建於程式中，使用 array 定義 `metrics`，然後呼叫 `nats-runner`。

修改下面 `CONFIG` 即可快速調整要發送的內容，然後直接執行：
  python scripts/run_with_metrics.py
"""
import json
import subprocess
from typing import List, Dict

# -------------------------
# 修改這裡的設定（內建參數）
# -------------------------
CONFIG = {
    "nats_runner": "./nats-runner",
    "template": "templates/srp_types.toml",
    "entry": "create_multi_metrics",
    "srp_type": "ai-service-package",
    "description": "AI 服務包",
    # resources 列表
    "resources": ["api", "compute"],
    # metrics 為 array，直接定義好多個 metric
    "metrics": [
        {"field": "aitoken", "type": "sum"},
        {"field": "qps", "type": "sum"},
    ],
}


def build_metrics_json(metrics: List[Dict[str, str]]) -> str:
    return json.dumps(metrics, separators=(",", ":"))


def build_resource_list(resources: List[str]) -> str:
    # template expects: ["r1","r2"] inserted as [{{resource_list}}]
    # we provide the inner items without outer brackets
    return ",".join(f'"{r}"' for r in resources)


def main():
    metrics_json = build_metrics_json(CONFIG["metrics"])
    resources_quoted = build_resource_list(CONFIG["resources"])

    cmd = [
        CONFIG["nats_runner"],
        "-t", CONFIG["template"],
        "-n", CONFIG["entry"],
        f"srp_type={CONFIG['srp_type']}",
        f"description={CONFIG['description']}",
        f"resource_list={resources_quoted}",
        f"metrics={metrics_json}",
    ]

    print("Command:", " ".join(cmd))
    subprocess.run(cmd, check=True)


if __name__ == "__main__":
    main()
