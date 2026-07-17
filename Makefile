.PHONY: fmt lint vet test cover bench tidy

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
