# OpenClaw NL2SQL UI 下载、登录和日志优化说明

## 本次优化目标

针对数据导出页面和容器排查体验做增量优化：

1. 数据导出文件不再通过浏览器新窗口直接打开，而是通过 UI 按钮触发文件下载。
2. 下载接口增加 `Content-Disposition: attachment`，即使用户直接访问下载 URL，也会优先作为附件下载。
3. 服务日志统一输出到容器终端 stdout，方便通过 `docker logs`、`kubectl logs` 直接排查。
4. 管理后台默认启用本地登录，默认账号密码为 `admin / admin`。
5. 管理后台登录状态保存在内存会话中，服务重启后立即失效；默认 12 小时自动过期。
6. SSO / OIDC 仍默认关闭，不影响后续接入。

## 下载体验变化

数据导出页面右侧“导出文件”列表中的下载操作已改为：

- 点击按钮后前端通过 `fetch` 获取文件。
- 使用浏览器 Blob URL 触发本地保存。
- 不再打开新标签页浏览 `.md` 或 `.json` 内容。
- 成功触发后 UI toast 提示“已开始下载”。

下载接口同时增加：

```http
Content-Disposition: attachment; filename="schema_xxx.md"
X-Content-Type-Options: nosniff
```

## 登录默认配置

默认配置：

```yaml
admin:
  auth:
    enabled: true
    session_ttl_hours: 12
    cookie_name: openclaw_admin_session
    admin_token_env: OPENCLAW_ADMIN_TOKEN
  sso:
    enabled: false
```

首次启动时，如果用户文件不存在，系统会自动创建本地管理员：

```text
用户名：admin
密码：admin
```

如需覆盖默认密码，启动前设置环境变量：

```bash
export OPENCLAW_ADMIN_INITIAL_PASSWORD='your-strong-password'
```

如果明确需要关闭管理后台登录，可在配置中设置：

```yaml
admin:
  auth:
    enabled: false
```

## 会话失效规则

- 会话只保存在进程内存中。
- 服务重启后，所有已登录会话立即失效，需要重新登录。
- 默认 12 小时自动过期，可通过 `admin.auth.session_ttl_hours` 调整。

## 容器日志

主进程启动时设置：

```go
log.SetOutput(os.Stdout)
log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
```

新增 HTTP 请求日志，格式示例：

```text
2026/06/13 15:20:31.123456 server.go:59: http method=POST path=/v1/admin/schema/export status=200 bytes=312 duration_ms=2814 remote=10.0.0.12:54321
```

新增关键事件日志：

- 管理员登录成功 / 失败
- 架构导出开始
- 架构导出完成，包含库、表、视图、字段、索引数量
- 导出文件下载
- 原有任务执行失败、SQL 执行失败、Telegram 发送失败等日志继续输出到终端

容器排查命令：

```bash
docker logs -f <container>
```

或 Kubernetes：

```bash
kubectl logs -f deploy/<deployment-name> -n <namespace>
```

## 建议上线步骤

1. 替换代码包并重新构建镜像。
2. 同步新的 `config.yaml`，确认 `admin.auth.enabled: true`。
3. 首次登录后进入“用户管理”，尽快修改默认 admin 密码或新增管理员账号。
4. 测试“数据导出”并点击右侧文件“下载”，确认浏览器不再打开新标签页。
5. 通过容器日志确认导出、下载、登录事件均正常输出。
