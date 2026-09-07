# EXALTED Terminal — build targets
#
#   make build         Build Linux binary (./bin/exalted)
#   make build-windows Cross-compile Windows binary (./dist/exalted.exe)
#   make build-all     Build Linux + Windows
#   make install       Install to ~/.local/bin
#   make gui           Build the graphical (Fyne) edition -> bin/exalted-gui
#   make appimage      Build a portable Linux AppImage -> dist/EXALTED_Terminal.AppImage
#   make apk           Build Android arm64 APK -> dist/exalted-<version>-android-arm64.apk
#   make test          Run unit tests
#   make tidy          gofmt + go mod tidy
#
# Binary name: exalted (EXALTED Terminal)

BINARY := exalted
VERSION ?= 0.2.0
LDFLAGS := -s -w -X main.version=$(VERSION)

# Android cross-build: NDK >= r19c; arm64 only (modern NDKs dropped old
# arm/386 clangs). The desktop CGO_* flags must be cleared so Homebrew's
# x86-64 libs never leak into the aarch64 link.
ANDROID_NDK_HOME ?= /var/home/Gigatone/Android/android-ndk-r27c
APK := dist/$(BINARY)-$(VERSION)-android-arm64.apk

GO      ?= go
GOOS    := $(shell $(GO) env GOOS)
GOARCH  := $(shell $(GO) env GOARCH)

PKG := ./cmd/biblelearn
GUIPKG := ./cmd/exalted-gui
GUIAPP := bin/exalted-gui
APPIMAGE := dist/EXALTED_Terminal-x86_64.AppImage
APPIMAGETOOL ?= /var/home/Gigatone/tools/appimagetool

.PHONY: build build-windows build-all install gui appimage apk test test-gui tidy clean

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

# GUI needs GL/Wayland/X11 dev headers (e.g. from Homebrew on Fedora Atomic).
gui:
	@mkdir -p bin
	./scripts/build-gui.sh
	@install -m 0755 /tmp/opencode/exalted-gui bin/exalted-gui
	@echo "built $(GUIAPP)"

appimage: gui
	./scripts/package-appimage.sh
	@echo "built $(APPIMAGE)"

apk:
	@mkdir -p dist
	@bash -c '. scripts/env.sh && \
	  export PATH="$$(go env GOPATH)/bin:$$PATH" && \
	  export ANDROID_NDK_HOME="$(ANDROID_NDK_HOME)" && \
	  unset CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_LDFLAGS PKG_CONFIG_PATH && \
	  fyne package -os android/arm64 \
	    --app-id com.gigatone.biblelearn \
	    --name $(BINARY) \
	    --icon $(abspath cmd/exalted-gui/icon.png) \
	    --source-dir $(abspath cmd/exalted-gui) \
	    --app-version $(VERSION) --app-build 1'
	@cp cmd/exalted-gui/$(BINARY).apk $(APK)
	@echo "built $(APK)"

test:
	$(GO) test $(GO_FLAGS) ./cmd/biblelearn/... ./internal/...
	@echo "note: GUI tests need the Homebrew env; run: make test-gui"

test-gui:
	@bash -c '. scripts/env.sh && go test ./cmd/exalted-gui/...'

tidy:
	$(GO) fmt ./...
	$(GO) mod tidy

clean:
	rm -rf bin dist
