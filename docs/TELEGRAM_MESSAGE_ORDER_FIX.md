# Telegram 消息顺序修复说明

## 问题

旧版本在 `Submit()` 中使用 goroutine 异步发送“已收到/排队执行”消息，同时 worker 可能非常快地完成 SQL 校验或执行并发送失败消息，导致 Telegram 中出现：

```text
查询任务失败 ...
已收到查询任务 ... 正在排队执行
```

这种顺序会让用户误判任务状态。

## 修复

新版本移除了 `Submit()` 中的异步通知，改为由 worker 在同一个任务流程中串行发送进度消息：

1. worker 开始处理任务后，先发送：

```text
已收到查询任务，任务ID：xxx，正在进行 SQL 安全校验，校验通过后会执行查询。
```

2. SQL 校验通过后，发送：

```text
SQL 安全校验通过，任务ID：xxx，数据源：xxx，正在执行查询。
```

3. 如果失败，只发送最终失败结果，并包含执行 SQL：

```text
查询任务失败：xxx
状态：SQL 结构错误：表或字段不存在
数据源：semantic_mart
错误原因：SQL 执行失败：Error 1105 ... Unknown column ...

执行 SQL：
SELECT ...
```

## 结果

同一个任务内不再并发发送 Telegram 消息，可避免“先失败，后排队”的乱序问题。

## 配置

仍然使用原配置项：

```yaml
queue:
  notify_on_accepted: true
```

含义调整为：是否发送任务进度消息。设置为 `false` 时，只发送最终结果或最终失败消息。
