# API Spec

## 鉴权

当前版本不启用 API Key/Bearer 鉴权。所有接口直接通过 HTTP 请求访问。建议只在内网、Kubernetes ClusterIP、OpenClaw 同 VPC 或受控网关后暴露。

## POST /v1/query-jobs

提交异步 SQL 查询任务。

### Request

```json
{
  "request_id": "tg-123456-001",
  "user_id": "123456",
  "chat_id": "123456",
  "session_id": "telegram:123456",
  "question": "最近7天按币种统计充值金额趋势",
  "data_source_id": "semantic_mart",
  "sql": "SELECT ... LIMIT 1000",
  "chart_hint": {
    "type": "line",
    "x": "dt",
    "y": ["recharge_amount"],
    "series": "currency"
  },
  "cache_key": "optional-idempotent-cache-key"
}
```

字段说明：

| 字段 | 必填 | 说明 |
|---|---|---|
| `request_id` | 否 | 幂等任务 ID，建议 OpenClaw 生成 |
| `user_id` | 是 | 当前 Telegram 用户 ID |
| `chat_id` | 是 | Telegram 私聊 chat_id，结果会发到这里 |
| `session_id` | 否 | 会话 ID |
| `question` | 否 | 原始问题，用于结果标题 |
| `data_source_id` | 否 | 不传则按 SQL 表名自动路由 |
| `sql` | 是 | 模型生成的 SQL |
| `chart_hint` | 否 | 图表建议 |
| `cache_key` | 否 | 自定义缓存键 |

### Response

```json
{
  "job_id": "tg-123456-001",
  "status": "queued"
}
```

## GET /v1/jobs/{job_id}

查询任务状态，只返回任务元信息，不返回查询结果。

## GET /v1/datasources

查看数据源状态、Host 冷却状态、并发配置。

## GET /readyz

检查所有数据源 Host 的 Ping 状态。

## Admin UI and Debug APIs

### GET /admin

返回管理后台 HTML 页面。页面支持任务列表、任务详情、重跑任务、手动 SQL 查询并发送给指定 Telegram 用户。

### GET /v1/admin/jobs

查询任务列表。

Query 参数：

- `status`：可选，按任务状态过滤。
- `q`：可选，按任务 ID、用户、SQL、错误、结果内容搜索。
- `limit`：可选，默认 50，最大 200。
- `offset`：可选，默认 0。

### GET /v1/admin/jobs/{job_id}

查询任务详情，包含原始 SQL、重写后的执行 SQL、结果预览、事件记录等。

### POST /v1/admin/jobs/{job_id}/rerun

重新执行指定任务，并使用原任务的 `chat_id` 发送结果给原用户。重跑任务会自动清空 `request_id` 和 `cache_key`。

### POST /v1/admin/sql/execute

手动执行 SQL。

请求体示例：

```json
{
  "data_source_id": "semantic_mart",
  "chat_id": "7997315413",
  "user_id": "7997315413",
  "question": "手动排查昨日美国VPBET充值提现",
  "sql": "SELECT ..."
}
```

如果 `chat_id` 为空但 `user_id` 不为空，会使用 `user_id` 作为 Telegram 私聊发送目标。
