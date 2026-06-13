# OpenClaw NL2SQL 管理后台 UI

本版本新增一个轻量级管理后台，方便排查 OpenClaw 触发的 NL2SQL 查询任务、查看执行 SQL、查看结果、重跑任务，以及手动输入 SQL 查询并私聊发送给指定 Telegram 用户。

## 访问入口

服务启动后访问：

```text
http://<host>:8088/admin
```

根路径 `/` 会自动跳转到 `/admin`。

## 页面能力

### 1. 任务管理

可以查看所有任务记录，包含：

- 任务 ID
- 状态：`queued / validating / running / sent / sent_cached / failed / rejected`
- 用户 ID / Chat ID
- 原始问题
- 原始 SQL 和安全校验后的实际执行 SQL
- 查询结果摘要
- 结果预览，最多保留前 100 行
- 事件记录，包括 queued、validating、running、result_ready、sent、failed 等

支持按任务 ID、用户、SQL、错误、结果内容搜索。

### 2. 重新执行任务

在任务列表或详情页点击“重跑”，后台会使用该任务的原始请求重新提交一个新任务：

- 使用原任务的 `chat_id`，执行完成后会发送给原用户。
- 自动清空 `request_id`，避免复用旧任务 ID。
- 自动清空 `cache_key`，避免命中缓存导致无法排查真实查询结果。

### 3. 手动 SQL 查询

在“手动查询”页可以输入：

- 数据源，支持自动路由或指定数据源。
- Telegram 用户 ID / Chat ID。
- 问题描述。
- SQL。

提交后会创建一个异步任务。执行完成后：

- 如果填写了用户 ID / Chat ID，会私聊发送结果。
- 无论是否填写用户 ID，都可以在管理后台任务详情里查看 SQL、结果和事件。

## 新增 API

### 查询任务列表

```http
GET /v1/admin/jobs?status=failed&q=vpbet&limit=50&offset=0
```

返回：

```json
{
  "total": 1,
  "jobs": [
    {
      "id": "...",
      "status": "failed",
      "question": "搜索美国VPBET的昨日总成功充值金额和总提现金额",
      "chat_id": "7997315413",
      "data_source_id": "semantic_mart",
      "rewritten_sql": "SELECT ...",
      "result_text": "...",
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

### 查询任务详情

```http
GET /v1/admin/jobs/{job_id}
```

返回完整 Job JSON，包含：

- `request.sql`：OpenClaw 提交的原始 SQL
- `rewritten_sql`：安全校验、补 LIMIT 后实际执行的 SQL
- `selected_tables`：识别出的真实表
- `result_text`：业务可读结果
- `result_columns / result_rows`：结果预览
- `events`：任务事件记录

### 重新执行任务

```http
POST /v1/admin/jobs/{job_id}/rerun
```

返回新的任务 ID：

```json
{
  "job_id": "new_job_id",
  "status": "queued"
}
```

### 手动执行 SQL

```http
POST /v1/admin/sql/execute
Content-Type: application/json

{
  "data_source_id": "semantic_mart",
  "chat_id": "7997315413",
  "user_id": "7997315413",
  "question": "手动排查昨日美国VPBET充值提现",
  "sql": "SELECT ..."
}
```

如果 `chat_id` 为空但填写了 `user_id`，服务会自动使用 `user_id` 作为私聊发送目标。

## Telegram 通知格式调整

本版本默认将通知压缩为只包含两段：

```text
🔎 查询语句
SELECT ...

📊 查询结果
昨日美国VPBET：总成功充值金额：100万；总提现金额：50万
```

默认关闭以下干扰项：

- 已收到任务通知
- SQL 校验通过通知
- 数据源、节点、返回行数等调试信息
- 自动图表 SVG
- 自动 CSV 附件

需要恢复旧模式时，可以在配置中调整：

```yaml
queue:
  notify_on_accepted: true
telegram:
  compact_result_only: false
  send_csv: true
  send_chart_svg: true
```

## 配置建议

为了方便调试并避免 `schema is not allowed` 这类 API 层白名单误伤，本版本仍建议：

```yaml
guard:
  allowed_schemas: []
  allowed_tables: []
  denied_tables: []
  enforce_known_tables: false
```

表、库、视图访问权限交给数据库账号本身控制。API 层只保留 SELECT-only、多语句禁止、危险函数禁止、大表时间过滤等安全保护。

## 安全建议

管理后台可以手动执行 SQL，生产环境建议至少满足一项：

- 仅部署在内网。
- 通过 Nginx / Ingress 加 IP 白名单。
- 通过反向代理增加 SSO、Basic Auth 或 VPN 访问控制。

当前代码保持与原 noauth 版本一致，没有额外加登录态，便于 OpenClaw 内部快速接入。
