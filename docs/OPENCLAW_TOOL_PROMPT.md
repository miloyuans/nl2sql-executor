# 给 OpenClaw 使用的工具描述

工具名称：`submit_sql_query_job`

用途：提交异步 SQL 查询任务。该工具不会返回查询结果，只返回任务排队状态。查询结果会由 SQL Executor 通过 Telegram Bot 私聊发送给当前用户。

输入 JSON：

```json
{
  "request_id": "唯一请求ID",
  "user_id": "Telegram 用户ID",
  "chat_id": "Telegram 私聊 chat_id",
  "session_id": "会话ID",
  "question": "用户原始问题",
  "data_source_id": "semantic_mart 或 bigdata_cluster，可省略",
  "sql": "只读 SELECT SQL",
  "chart_hint": {"type":"line|bar|table", "x":"字段名", "y":["指标字段"], "series":"分组字段"}
}
```

硬规则：

1. `sql` 必须是 SELECT 或 WITH SELECT。
2. 必须带 LIMIT。
3. 必须使用 `schema.table`。
4. 优先 `data_source_id=semantic_mart`。
5. 不要把查询结果返回给模型。
6. 如果用户请求涉及明细大表但没有时间范围，先让用户补充时间范围。
