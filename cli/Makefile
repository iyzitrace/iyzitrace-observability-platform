BIN              ?= iyzitrace
OUT              ?= bin/$(BIN)
VERSION          ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT           ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE             ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUNDLE_VERSION   ?= $(shell cat ../bundle/BUNDLE_VERSION 2>/dev/null | tr -d ' \n\t' || echo dev)
PKG              := github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/cli
LDFLAGS          := -s -w \
                    -X '$(PKG).BinaryVersion=$(VERSION)' \
                    -X '$(PKG).BinaryCommit=$(COMMIT)' \
                    -X '$(PKG).BinaryDate=$(DATE)' \
                    -X '$(PKG).DefaultBundleVersion=$(BUNDLE_VERSION)'

.PHONY: build test tidy clean run

build:
	go build -ldflags "$(LDFLAGS)" -o $(OUT) ./cmd/iyzitrace

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/

run: build
	./$(OUT) --help
