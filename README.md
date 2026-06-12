# nl2sql-executor-go-prod

面向 OpenClaw / Telegram Bot / Doris 或 MySQL 协议集群的异步 SQL 执行服务。

核心目标：OpenClaw 只负责自然语言转 SQL 并提交任务；本工具在内部异步执行 SQL，并把结果直接通过 Telegram Bot 私聊发送给用户。查询结果不会返回给 OpenClaw，也不会进入大模型上下文。

## 功能

- Go HTTP API，支持 OpenClaw 以 API 方式提交查询任务
- 多数据源配置：每个数据源支持多 Host、多连接池、独立并发和 SQL Guard
- 多节点负载均衡：每次查询动态轮转节点，失败自动切换下一个节点
- Host 熔断：连续失败后进入冷却，避免持续打坏节点
- 异步任务队列：立即返回 job_id，后台执行并 Telegram 私聊结果
- SQL 安全校验：只允许 SELECT / WITH SELECT、禁止 DDL/DML、多语句、危险函数
- 大表保护：可配置大表强制 WHERE 和时间字段过滤
- 本地缓存：重复 SQL 可以命中缓存，降低数据库压力
- 大结果处理：内联消息自动拆分，完整结果 CSV，超过阈值自动 gzip
- 简单 SVG 图表：按 chart_hint 输出柱状/折线 SVG 文件

## 快速启动

```bash
export DB_USER='your_db_user'
export DB_PASSWORD='your_db_password'
export TELEGRAM_BOT_TOKEN='123456:telegram-token'

cp configs/config.example.yaml configs/config.yaml
go mod tidy
go build -o bin/nl2sql-executor ./cmd/nl2sql-executor
CONFIG_PATH=configs/config.yaml ./bin/nl2sql-executor
```

## OpenClaw 调用示例

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

API 只返回：

```json
{"job_id":"tg-123456-001","status":"queued"}
```

查询结果由服务直接发送给 Telegram `chat_id`。

## 生产建议

1. 第一阶段只开放 `semantic_mart`、`global_dm`、`international-data`。
2. 大明细表放入 `denied_tables` 或 `large_tables`。
3. `bigdata_cluster.execution.max_concurrency` 初始设为 1。
4. 所有 OpenClaw 生成的 SQL 必须使用 `schema.table`。
5. 为高频跨库关联查询创建 `ai_mart` 视图或物理宽表，再让模型优先查询 `ai_mart`。

## 容器部署模板

已内置部署模板：

- `deploy/docker-compose/`：单机容器部署。
- `deploy/kubernetes/`：Kubernetes Deployment / Service / PVC / ConfigMap / Secret 模板。
- `deploy/README_DEPLOYMENT.md`：完整部署说明。

Kubernetes 内部调用地址示例：

```text
http://nl2sql-executor.nl2sql.svc.cluster.local:8088/v1/query-jobs
```


## 无鉴权接口说明

当前版本已按需求移除 API Key/Bearer 鉴权。`POST /v1/query-jobs`、`GET /v1/datasources`、`GET /v1/jobs/{job_id}` 可直接从内网调用。生产部署时请通过 Kubernetes NetworkPolicy、Ingress 白名单、内网 Service 或网关 ACL 控制访问范围，避免公网暴露。
