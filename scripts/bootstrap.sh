#!/usr/bin/env bash
set -euo pipefail

: "${NL2SQL_EXECUTOR_API_KEY:?need NL2SQL_EXECUTOR_API_KEY}"
: "${DB_USER:?need DB_USER}"
: "${DB_PASSWORD:?need DB_PASSWORD}"
: "${TELEGRAM_BOT_TOKEN:?need TELEGRAM_BOT_TOKEN}"

cp -n configs/config.example.yaml configs/config.yaml || true
go mod tidy
go build -o bin/nl2sql-executor ./cmd/nl2sql-executor
CONFIG_PATH=configs/config.yaml ./bin/nl2sql-executor
