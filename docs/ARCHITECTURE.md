# 架构说明

## 目标

该服务用于承接 OpenClaw / Telegram Bot 触发的 SQL 查询任务。OpenClaw 负责将自然语言转换成 SQL；本服务负责异步执行、安全校验、限流、缓存、格式化和 Telegram 私聊发送。

## 链路

```text
Telegram 用户
  -> OpenClaw / AI 模型生成 SQL
  -> POST /v1/query-jobs
  -> 本服务入队并立即返回 202
  -> worker 异步处理
  -> Datasource Router 选择数据源
  -> SQL Guard 校验和改写
  -> Datasource Manager 限流执行
  -> Formatter 生成文本/CSV/GZIP/SVG
  -> Telegram Bot 私聊用户
```

## 多数据源

数据源是逻辑查询入口，而不是一个物理 Host。每个数据源可配置：

- 多 Host
- 独立连接池
- 独立最大并发
- 独立查询超时
- 独立 SQL Guard
- 独立大表策略

建议：

- `semantic_mart`：AI 问数优先入口，承载 `ai_mart` 视图、物理宽表和 DM 聚合表。
- `bigdata_cluster`：原始大数据集群入口，强限流，主要给受控场景查询。

## 节点故障切换

每次查询会轮询当前数据源下的 Host。某 Host 查询失败后会记录失败次数；连续失败达到阈值后进入冷却期。冷却期内跳过该 Host。

该机制是应用层负载均衡，不依赖 JDBC loadbalance URL。

## 压力保护

- API 层：队列 buffer 限制
- 用户层：`max_per_user_running`
- 数据源层：`execution.max_concurrency`
- 数据库层：连接池 `max_open_conns`
- SQL 层：LIMIT、max_rows、大表 WHERE、时间字段要求
- 结果层：只返回有限行数，完整文件通过 CSV/GZIP

## 安全边界

本工具不信任 OpenClaw/模型生成的 SQL。所有 SQL 必须经过本地 SQL Guard：

- 只允许 SELECT / WITH SELECT
- 禁止多语句
- 禁止 DDL/DML/权限/文件相关语句
- 禁止危险函数
- 表必须 schema-qualified
- 可选 schema catalog 校验
- 大表必须有时间过滤

## 结果不返回模型

`POST /v1/query-jobs` 只返回 job_id 和状态。查询结果不作为 API 响应返回，避免结果进入 OpenClaw/AI 模型上下文。
