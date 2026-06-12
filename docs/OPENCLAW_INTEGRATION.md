# OpenClaw 对接建议

## 原则

OpenClaw / AI 模型只做：

1. 理解用户问题
2. 结合 schema prompt 生成 SQL
3. 选择 `data_source_id`
4. 调用本服务 `/v1/query-jobs`
5. 告诉用户“任务已提交，结果会私聊发送”

OpenClaw 不接收 SQL 执行结果。

## 模型输出 JSON 建议

```json
{
  "status": "ok",
  "data_source_id": "semantic_mart",
  "sql": "SELECT ... LIMIT 1000",
  "chart": {
    "type": "line",
    "x": "day",
    "y": ["recharge_amount"],
    "series": "currency"
  },
  "notes": []
}
```

## OpenClaw 工具调用体

```json
{
  "request_id": "{{message.platform}}-{{message.chat_id}}-{{message.id}}",
  "user_id": "{{message.from.id}}",
  "chat_id": "{{message.chat_id}}",
  "session_id": "{{message.platform}}:{{message.chat_id}}",
  "question": "{{message.text}}",
  "data_source_id": "{{model_output.data_source_id}}",
  "sql": "{{model_output.sql}}",
  "chart_hint": "{{model_output.chart}}"
}
```

## 提交后回复用户

```text
查询任务已提交，结果会稍后通过私聊发送。
```

## Prompt 约束

给 OpenClaw 的系统提示词应包含：

```text
你只能生成只读 SELECT SQL。必须使用真实存在的 schema.table 和字段。优先查询 ai_mart、global_dm、international-data 聚合表。不要直接查询大明细表；如必须查询大表，必须带时间条件和 LIMIT。输出 JSON，不要解释。执行结果由外部 SQL Executor 直接发给 Telegram 用户，不要要求工具返回查询结果。
```


> 当前版本已移除 API Key/Bearer 鉴权，接口可直接 POST 调用。请勿公网裸露服务，建议仅通过内网 Service、OpenClaw 所在网段、Ingress 白名单或网关 ACL 访问。
