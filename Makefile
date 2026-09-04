GO ?= go
ARCH := $(shell $(GO) env GOARCH)
.PHONY: test build package
test:
	$(GO) test -race ./...
	$(GO) vet ./...
build:
	mkdir -p dist
	CGO_ENABLED=1 $(GO) build -trimpath -buildmode=c-shared -o dist/miftah.so ./cmd/plugin
package: test build
	cd dist && zip -j miftah_0.1.5_linux_$(ARCH).zip miftah.so
	cd dist && sha256sum miftah_0.1.5_linux_$(ARCH).zip > checksums.txt
