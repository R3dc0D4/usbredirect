.PHONY: build build-all clean test

BINARY=usbredirect
VERSION=$(shell grep 'version = ' cmd/usbredirect/main.go | awk -F'"' '{print $$2}')
GO=go
GORELEASER?=goreleaser

build:
	$(GO) build -o $(BINARY) ./cmd/usbredirect

build-all:
	GOOS=linux GOARCH=amd64 $(GO) build -o dist/$(BINARY)-linux-amd64 ./cmd/usbredirect
	GOOS=windows GOARCH=amd64 $(GO) build -o dist/$(BINARY)-windows-amd64.exe ./cmd/usbredirect
	GOOS=darwin GOARCH=amd64 $(GO) build -o dist/$(BINARY)-darwin-amd64 ./cmd/usbredirect
	GOOS=darwin GOARCH=arm64 $(GO) build -o dist/$(BINARY)-darwin-arm64 ./cmd/usbredirect

clean:
	rm -f $(BINARY) dist/*

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

lint:
	golangci-lint run ./...

run-server: build
	./$(BINARY) agent --mode server --port /dev/ttyUSB0 --baud 9600 --listen :5760

run-client: build
	./$(BINARY) agent --mode client --remote 127.0.0.1:5760 --virtual /tmp/ttyV0

run-ports: build
	./$(BINARY) ports

release:
	$(GORELEASER) release --clean

version:
	@echo "$(VERSION)"