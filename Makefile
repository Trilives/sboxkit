.PHONY: test vet build deb clean

BIN := sboxkit
GO ?= go
GOCACHE ?= $(CURDIR)/.tools/go-build
GOMODCACHE ?= $(CURDIR)/.tools/go-mod
VERSION ?= $(shell cat VERSION)
SING_BOX_BIN ?=

test:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test ./...

vet:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) vet ./...

build:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -o $(BIN) ./cmd/sboxkit

deb: build
	@test -n "$(SING_BOX_BIN)" || (echo "SING_BOX_BIN=/path/to/sing-box is required for make deb" >&2; exit 2)
	packaging/deb/build-deb.sh --binary $(BIN) --sing-box $(SING_BOX_BIN) --version $(VERSION) --arch amd64 --out-dir dist

clean:
	rm -f $(BIN)
