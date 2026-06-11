FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -o /out/nl2sql-executor ./cmd/nl2sql-executor

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=builder /out/nl2sql-executor /app/nl2sql-executor
COPY configs /app/configs
COPY data/schema /app/data/schema
RUN mkdir -p /app/data/cache /app/data/results /app/data/jobs && chown -R appuser:appuser /app
USER appuser
EXPOSE 8088
ENV CONFIG_PATH=/app/configs/config.example.yaml
CMD ["/app/nl2sql-executor"]
