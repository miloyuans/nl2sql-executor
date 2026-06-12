# No-auth API mode

当前版本已移除 API Key / Bearer 鉴权。

可直接调用：

```bash
curl -X POST http://127.0.0.1:8088/v1/query-jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "request_id":"tg-123456-001",
    "user_id":"123456",
    "chat_id":"123456",
    "session_id":"telegram:123456",
    "question":"最近7天按币种统计充值金额趋势",
    "data_source_id":"semantic_mart",
    "sql":"SELECT user_event_date AS dt, currency, SUM(recharge_amount_total) AS recharge_amount FROM `global_dm`.`dm_user_event_currency_summary_min` WHERE user_event_date >= DATE_SUB(CURDATE(), INTERVAL 7 DAY) GROUP BY user_event_date, currency ORDER BY dt, currency LIMIT 1000",
    "chart_hint":{"type":"line","x":"dt","y":["recharge_amount"],"series":"currency"}
  }'
```

返回：

```json
{"job_id":"tg-123456-001","status":"queued"}
```

注意：无鉴权模式只能部署在内网或受控网关后。不要公网裸露该服务。
建议至少使用以下一种访问控制：

- Kubernetes ClusterIP，仅允许 OpenClaw 所在命名空间访问。
- NetworkPolicy 限制来源 Pod/namespace。
- Ingress / API Gateway IP 白名单。
- 防火墙只允许 OpenClaw 服务器访问 8088。
