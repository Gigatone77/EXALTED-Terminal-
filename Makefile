# EXALTED Terminal — build targets
#
#   make build         Build Linux binary (./bin/exhaled)
#   make build-windows Cross-compile Windows binary (./dist/exhaled.exe)
#   make build-all     Build Linux + Windows
#   make install       Install to ~/.local/bin
#   make test          Run unit tests
#   make tidy          gofmt + go mod tidy
#
# Binary name: exhaled (EXALTED Terminal)

BINARY := exhaled
VERSION ?= 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)

GO      ?= go
GOOS    := $(shell $(GO) env GOOS)
GOARCH  := $(shell $(GO) env GOARCH)

PKG := ./cmd/biblelearn

.PHONY: build build-windows build-all install test tidy clean

build:
	@mkdir -p bin
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)
	@echo "built bin/$(BINARY) ($(GOOS)/$(GOARCH))"

build-windows:
	@mkdir -p dist
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY).exe $(PKG)
	@echo "built dist/$(BINARY).exe (windows/amd64)"

build-linux:
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 $(PKG)
	@echo "built dist/$(BINARY)-linux-amd64"

build-all: build-linux build-windows

install: build
	install -m 0755 bin/$(BINARY) $(HOME)/.local/bin/$(BINARY)
	@echo "installed $(HOME)/.local/bin/$(BINARY)"

test:
	$(GO) test ./...

tidy:
	$(GO) fmt ./...
	$(GO) mod tidy

clean:
	rm -rf bin dist
