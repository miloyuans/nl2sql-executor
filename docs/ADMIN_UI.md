# OpenClaw NL2SQL 管理后台升级说明

本版本将 `/admin` 管理后台升级为暗色侧边栏风格，参考 `gw-ipinfo-nginx` 管理后台的导航、卡片、表格和表单设计。后台默认仍保持 no-auth 兼容模式，不影响 OpenClaw 现有调用。

## 页面入口

- 管理后台：`/admin`
- 本地登录页：`/admin/login`，仅 `admin.auth.enabled: true` 时启用
- SSO 登录：`/admin/sso/login`，仅 `admin.sso.enabled: true` 且配置完整时启用

## 核心功能窗口

### 1. 手动查询

用于手动输入 SQL 执行查询。可选择数据源、填写问题描述和 Request ID，也可以填写 Telegram 用户 ID / Chat ID。填写后，查询完成会按现有 Telegram 输出模板私聊发送给用户。

### 2. 任务列表

展示所有任务事件记录，包括：

- 任务 ID
- 状态
- 用户 / Chat ID
- 原始 SQL
- 重写后的 SQL
- 查询结果摘要
- 查询结果预览
- 事件流水
- 错误信息

支持点击“重新执行并发送给原用户”，用于排查结果为 0、字段错误、路由错误等问题。

### 3. 数据导出

一键导出当前数据库账号可访问的数据结构信息，输出 JSON 和 Markdown 两种格式。导出内容包括：

- 数据库 / schema
- 表
- 视图
- 字段
- 字段类型
- 字段注释
- 表注释
- 索引
- 视图定义

用途：将 Markdown 文件给 OpenClaw / AI 学习，JSON 文件用于程序化 RAG 更新，以便数据库结构、字段备注变化后及时更新 NL2SQL 的生成准确率。

### 4. 系统设置

支持在 UI 中查看和运行时调整：

- Telegram 输出是否仅保留“查询语句 + 查询结果”
- 是否自动发送 CSV
- 是否自动发送 SVG 图表
- 最大内联结果行数
- 架构导出目录和最大导出行数
- SSO / OIDC 配置

注意：UI 保存的是当前进程运行时配置。如需重启后仍生效，请同步修改 `config.yaml`。

### 5. 用户管理

用于维护本地管理后台账号。SSO 用户仍由身份提供商管理。

## 新增配置

```yaml
admin:
  auth:
    enabled: false
    session_ttl_hours: 12
    cookie_name: openclaw_admin_session
    admin_token_env: OPENCLAW_ADMIN_TOKEN
  sso:
    enabled: false
    issuer_url: ''
    client_id: ''
    client_secret: ''
    redirect_url: ''
    scopes: openid profile email
    admin_users: []
    admin_roles:
    - admin
    - openclaw-admin
    user_roles:
    - user
    - openclaw-user
  users:
    file: ./data/admin/users.json
  schema_export:
    dir: ./data/schema_exports
    max_rows: 200000
    include_system_schemas: false
    system_schemas:
    - information_schema
    - mysql
    - performance_schema
    - sys
    - __internal_schema
```

## SSO 行为

SSO 默认关闭。开启后使用 OIDC Discovery：

```text
{issuer_url}/.well-known/openid-configuration
```

支持标准 Authorization Code Flow。SSO 登录成功后，根据 `admin_users` 或 `admin_roles` 判断是否为管理员。

## 新增 API

- `GET /v1/admin/auth/me`
- `POST /v1/admin/auth/login`
- `POST /v1/admin/auth/logout`
- `GET /v1/admin/users`
- `POST /v1/admin/users`
- `DELETE /v1/admin/users/{username}`
- `GET /v1/admin/settings`
- `POST /v1/admin/settings`
- `POST /v1/admin/schema/export`
- `GET /v1/admin/schema/exports`
- `GET /v1/admin/schema/download?file=...`

## 数据导出接口示例

```bash
curl -X POST http://127.0.0.1:8088/v1/admin/schema/export \
  -H 'Content-Type: application/json' \
  -d '{"data_source_id":"bigdata_cluster","include_system_schemas":false}'
```

返回结果包含导出摘要和下载地址：

```json
{
  "summary": {
    "database_count": 12,
    "table_count": 1200,
    "view_count": 80,
    "column_count": 32000,
    "index_count": 5000
  },
  "json_file": "schema_bigdata_cluster_20260613_123000.json",
  "markdown_file": "schema_bigdata_cluster_20260613_123000.md"
}
```
