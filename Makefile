BINARY  := bin/lim-tools-server
PKG     := ./cmd/server
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: help run build test test-race lint fmt tidy clean

help: ## 显示所有可用目标
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | sed 's/:.*## /  --  /'

run: ## 本地启动服务
	go run $(PKG)

build: ## 编译到 bin/，版本号从 git 注入
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

test: ## 跑测试
	go test ./...

test-race: ## 带竞态检测跑测试
	go test -race ./...

lint: ## 静态检查（需要 golangci-lint）
	golangci-lint run

fmt: ## 格式化
	gofmt -s -w .

tidy: ## 整理依赖
	go mod tidy

clean: ## 清掉构建产物
	rm -rf bin
