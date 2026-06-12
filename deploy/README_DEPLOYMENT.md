# nl2sql-executor 容器部署模板

本目录提供两种部署方式：

- `docker-compose/`：单机或测试环境使用。
- `kubernetes/`：Kubernetes 生产/准生产环境使用。

服务监听端口：`8088`。

健康检查：

- `/healthz`：无需鉴权，用于 liveness/startup/readiness 基础探针。
- `/readyz`：需要 API Key，会检查数据源；生产环境可手动调用。

## 关键安全建议

1. 生产环境默认 `replicas: 1`，先不要直接水平扩容。每个 Pod 都会有独立数据库并发限制，扩容后总数据库并发会变成 `replicas * 每数据源 max_concurrency`。
2. 数据库账号必须是只读账号。
3. API 只暴露给 OpenClaw 所在内网，不建议公网开放。
4. 如果必须公网访问，请通过 Ingress、WAF、IP 白名单和 HTTPS 暴露。
5. `NL2SQL_EXECUTOR_API_KEY`、`DB_PASSWORD`、`TELEGRAM_BOT_TOKEN` 必须放 Secret，不要写进镜像。
6. `data/schema` 已打进镜像，PVC 只挂载 `cache/results/jobs`，不要把 PVC 直接挂到 `/app/data`，否则会覆盖镜像内置 schema 文件。

## Kubernetes 部署

### 1. 构建并推送镜像

```bash
docker build -t registry.example.com/nl2sql-executor:latest .
docker push registry.example.com/nl2sql-executor:latest
```

把 `deploy/kubernetes/04-deployment.yaml` 中的镜像改成你的真实镜像地址。

### 2. 修改 Secret

```bash
cp deploy/kubernetes/01-secret.example.yaml deploy/kubernetes/01-secret.yaml
vi deploy/kubernetes/01-secret.yaml
```

生产环境建议用集群 Secret 管理系统，不要把真实 Secret 提交到 Git。

### 3. 修改 ConfigMap

按你的环境修改：

```bash
vi deploy/kubernetes/02-configmap.yaml
```

重点检查：

- 数据源 hosts
- 每数据源 `max_concurrency`
- 允许 schema
- 大表时间条件
- Telegram 配置

### 4. 部署

如果使用示例 Secret 文件名，需要把 kustomization 中的 Secret 文件指向真实文件，或者直接 apply：

```bash
kubectl apply -f deploy/kubernetes/00-namespace.yaml
kubectl apply -f deploy/kubernetes/01-secret.yaml
kubectl apply -f deploy/kubernetes/02-configmap.yaml
kubectl apply -f deploy/kubernetes/03-pvc.yaml
kubectl apply -f deploy/kubernetes/04-deployment.yaml
kubectl apply -f deploy/kubernetes/05-service.yaml
```

也可以使用 kustomize：

```bash
kubectl apply -k deploy/kubernetes
```

### 5. 检查

```bash
kubectl -n nl2sql get pods
kubectl -n nl2sql logs -f deploy/nl2sql-executor
kubectl -n nl2sql port-forward svc/nl2sql-executor 8088:8088
curl http://127.0.0.1:8088/healthz
curl -H "Authorization: Bearer <你的 API Key>" http://127.0.0.1:8088/v1/datasources
```

## OpenClaw 调用地址

在同一个 Kubernetes 集群内：

```text
http://nl2sql-executor.nl2sql.svc.cluster.local:8088/v1/query-jobs
```

如果 OpenClaw 不在集群内，可以开启 Ingress 或通过内网负载均衡暴露 Service。

## Docker Compose 部署

```bash
cd deploy/docker-compose
cp .env.example .env
vi .env
cd ../..
docker compose -f deploy/docker-compose/docker-compose.yml up -d --build
```

检查：

```bash
curl http://127.0.0.1:8088/healthz
curl -H "Authorization: Bearer <你的 API Key>" http://127.0.0.1:8088/v1/datasources
```

## 提交测试任务

```bash
curl -X POST http://127.0.0.1:8088/v1/query-jobs \
  -H "Authorization: Bearer <你的 API Key>" \
  -H "Content-Type: application/json" \
  -d '{
    "request_id": "test-001",
    "user_id": "123456",
    "chat_id": "123456",
    "session_id": "telegram:123456",
    "question": "最近7天按币种统计充值金额趋势",
    "data_source_id": "semantic_mart",
    "sql": "SELECT user_event_date AS dt, currency, SUM(recharge_amount_total) AS recharge_amount FROM `global_dm`.`dm_user_event_currency_summary_min` WHERE user_event_date >= DATE_SUB(CURDATE(), INTERVAL 7 DAY) GROUP BY user_event_date, currency ORDER BY dt, currency LIMIT 1000",
    "chart_hint": {"type":"line", "x":"dt", "y":["recharge_amount"], "series":"currency"}
  }'
```

注意：`chat_id` 必须是 Bot 可以私聊到的 Telegram 用户/会话 ID。
