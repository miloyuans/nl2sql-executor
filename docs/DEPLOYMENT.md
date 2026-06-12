# 部署建议

## systemd 示例

```ini
[Unit]
Description=NL2SQL Executor
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/nl2sql-executor
Environment=CONFIG_PATH=/opt/nl2sql-executor/configs/config.yaml
Environment=DB_USER=readonly_user
Environment=DB_PASSWORD=change-me
Environment=TELEGRAM_BOT_TOKEN=change-me
ExecStart=/opt/nl2sql-executor/bin/nl2sql-executor
Restart=always
RestartSec=3
User=nl2sql
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/opt/nl2sql-executor/data

[Install]
WantedBy=multi-user.target
```

## Nginx 内网反代建议

只允许 OpenClaw 所在机器访问 `/v1/query-jobs`，不要暴露公网。

## 初始限流建议

```yaml
semantic_mart.execution.max_concurrency: 2
bigdata_cluster.execution.max_concurrency: 1
queue.max_per_user_running: 1
```

上线后根据 Doris/StarRocks FE/BE 监控逐步调大。


> 当前版本已移除 API Key/Bearer 鉴权，接口可直接 POST 调用。请勿公网裸露服务，建议仅通过内网 Service、OpenClaw 所在网段、Ingress 白名单或网关 ACL 访问。
