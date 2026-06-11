APP=nl2sql-executor

.PHONY: tidy fmt test build run clean

tidy:
	go mod tidy

fmt:
	gofmt -w cmd internal

test:
	go test ./...

build:
	mkdir -p bin
	go build -o bin/$(APP) ./cmd/nl2sql-executor

run:
	CONFIG_PATH=configs/config.example.yaml go run ./cmd/nl2sql-executor

clean:
	rm -rf bin data/cache data/results data/jobs
