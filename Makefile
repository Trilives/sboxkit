BIN     := sboxkit
VERSION := $(shell cat VERSION 2>/dev/null || echo dev)
GO      ?= go

.PHONY: build test vet fmt release-snapshot

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/sboxkit

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w cmd internal

release-snapshot:
	goreleaser release --snapshot --clean
