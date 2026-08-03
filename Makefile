.PHONY: build vet test lint check

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./...

lint:
	golangci-lint run

check: vet build test lint
