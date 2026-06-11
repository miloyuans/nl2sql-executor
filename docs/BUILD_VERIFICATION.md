# Build verification

已执行：

```bash
gofmt -w cmd internal
```

当前 ChatGPT 执行环境无法访问 `proxy.golang.org`，所以没有在本环境完成 `go mod tidy` / `go test ./...` 的依赖下载步骤。项目依赖只有：

- `github.com/go-sql-driver/mysql v1.8.1`
- `gopkg.in/yaml.v3 v3.0.1`

在可联网或有内部 Go module proxy 的构建机上执行：

```bash
go mod tidy
go test ./...
go build -o bin/nl2sql-executor ./cmd/nl2sql-executor
```

如果在内网环境无法访问官方 Go proxy，可配置企业代理或国内代理，例如：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```
