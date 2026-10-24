.PHONY: build test lint clean release

GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X main.version=$(VERSION)

# Default target: build the plugin for the current platform.
build:
	CGO_ENABLED=1 $(GO) build -buildmode=c-shared -ldflags "$(LDFLAGS)" -o model-playground.so .

# Run all tests with race detector.
test:
	$(GO) test -race -count=1 ./...

# Lint everything (requires gofmt + go vet; staticcheck/gocritic/unparam optional).
lint:
	@test -z "$$($(GO)fmt -l .)" || ($(GO)fmt -l . && exit 1)
	$(GO) vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; fi
	@if command -v gocritic >/dev/null 2>&1; then gocritic check ./...; fi
	@if command -v unparam >/dev/null 2>&1; then unparam ./...; fi

# Clean build artifacts.
clean:
	rm -f model-playground.so model-playground.h
	rm -rf bin/ dist/ coverage.out coverage.html

# Cross-compile for all supported platforms (requires cross C toolchains).
release: clean
	@mkdir -p dist
	@for target in linux/amd64 linux/arm64; do \
	  os=$${target%/*}; arch=$${target#*/}; \
	  out=dist/model-playground_$(VERSION)_$${os}_$${arch}; \
	  mkdir -p $$out; \
	  GOOS=$$os GOARCH=$$arch CGO_ENABLED=1 $(GO) build -buildmode=c-shared -ldflags "$(LDFLAGS)" -o $$out/model-playground.so . 2>&1 | grep -v "cgo is not enabled" || true; \
	  if [ -f $$out/model-playground.so ]; then \
	    cd dist && zip -qr model-playground_$(VERSION)_$${os}_$${arch}.zip model-playground_$(VERSION)_$${os}_$${arch} && cd ..; \
	    echo "built $$out"; \
	  fi; \
	done
