# Docker Compose 部署

## 1. 准备环境变量

```bash
cd deploy/docker-compose
cp .env.example .env
vi .env
```

必须修改：

```bash
NL2SQL_EXECUTOR_API_KEY=你的内部 API Key
DB_USER=数据库只读账号
DB_PASSWORD=数据库密码
TELEGRAM_BOT_TOKEN=Telegram Bot Token
```

## 2. 启动

在项目根目录执行：

```bash
docker compose -f deploy/docker-compose/docker-compose.yml up -d --build
```

## 3. 检查

```bash
curl http://127.0.0.1:8088/healthz
curl -H "Authorization: Bearer $NL2SQL_EXECUTOR_API_KEY" http://127.0.0.1:8088/v1/datasources
```

## 4. 查看日志

```bash
docker compose -f deploy/docker-compose/docker-compose.yml logs -f --tail=200
```
