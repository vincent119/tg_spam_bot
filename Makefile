DOCKER_REGISTRY ?= docker.io
DOCKER_USERNAME ?= vincent119
DOCKER_IMAGE ?= $(DOCKER_REGISTRY)/$(DOCKER_USERNAME)/tg_spam_bot
DOCKER_TAG ?= latest
CONFIG_FILE ?= configs/config.yaml

.PHONY: fmt lint vet test cover bench tidy run docker-login docker-build docker-push docker-publish

# run 使用結構化範例設定啟動服務；秘密值仍須由環境變數提供。
run:
	CONFIG_FILE=$(CONFIG_FILE) go run ./cmd/tg-spam-bot

fmt:
	gofmt -s -w .

lint:
	golangci-lint run ./...

vet:
	go vet ./...

test:
	go test -race -count=1 ./...

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

bench:
	go test -run=NONE -bench=. -benchmem ./...

tidy:
	go mod tidy

# docker-login 使用 Docker CLI 的安全互動登入，不保存密碼於 Makefile。
docker-login:
	docker login $(DOCKER_REGISTRY) --username $(DOCKER_USERNAME)

# docker-build 建立指定版本映像；可用 DOCKER_TAG 覆寫標籤。
docker-build:
	docker build --tag $(DOCKER_IMAGE):$(DOCKER_TAG) .

# docker-push 推送已建立的指定版本映像。
docker-push:
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

# docker-publish 依序建置及推送映像，登入需先獨立完成。
docker-publish: docker-build docker-push
