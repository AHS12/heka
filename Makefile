# Heka build tooling — single entry point for dev, build, and checks (SPEC-01 §4).

VERSION   ?= 0.8.2
BIN_DIR   := build
DIST_DIR  := $(BIN_DIR)/dist
LDFLAGS   := -X main.appVersion=$(VERSION)

.PHONY: dev dev-core build test check vet lint format clean
.PHONY: release-dev release release-windows release-linux release-mac

ifeq ($(OS),Windows_NT)
EXE := .exe
else
EXE :=
endif

BIN     := $(BIN_DIR)/bin/heka$(EXE)
GUI_BIN := $(BIN_DIR)/bin/heka-gui$(EXE)

# Go tool binaries (wails, …) install to GOPATH/bin; prepend it so every
# target resolves them even when the ambient PATH predates the install.
GOBIN := $(shell go env GOPATH)/bin
ifeq ($(OS),Windows_NT)
PATH := $(GOBIN);$(PATH)
else
PATH := $(GOBIN):$(PATH)
endif

## dev: run the daemon and GUI together for development (Ctrl-C stops both)
dev:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev.ps1

## dev-core: run only the daemon in the foreground (own terminal)
dev-core: frontend/dist
	go run . daemon

## build: build the console binary; on Windows also the GUI-subsystem flavor
build:
ifeq ($(OS),Windows_NT)
	wails build -o heka-gui$(EXE) -ldflags "$(LDFLAGS)"
	wails build -windowsconsole -o heka$(EXE) -ldflags "$(LDFLAGS)"
else
	wails build -ldflags "$(LDFLAGS)"
endif

## test: Go tests + frontend suite (builds the frontend first so the go:embed works)
test: frontend/dist frontend-test
	go test ./...

## frontend-test: Vitest suite for the shell and pages (SPEC-12 §6)
frontend-test:
	cd frontend && npm test

## check: quality gate = vet + lint + test
check: vet lint test

vet:
	go vet ./...

lint:
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -Command "if (Get-Command golangci-lint -ErrorAction SilentlyContinue) { golangci-lint run --timeout 5m ./... } else { Write-Host 'golangci-lint not found — install from https://golangci-lint.run/' }"
else
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout 5m ./...; \
	else \
		echo "golangci-lint not found — install from https://golangci-lint.run/"; \
	fi
endif

## format: gofmt everything
format:
	go fmt ./...

## clean: remove generated outputs
clean:
	go clean
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -Command "Remove-Item -Recurse -Force -ErrorAction SilentlyContinue build, frontend/dist, .heka-dev-daemon.log, .heka-dev-daemon.log.err; exit 0"
else
	rm -rf build frontend/dist .heka-dev-daemon.log .heka-dev-daemon.log.err
endif

# ─── Release targets ─────────────────────────────────────────────────────────

# Cross-platform mkdir -p
ifeq ($(OS),Windows_NT)
MKDIR_P = @if not exist "$(1)" mkdir "$(1)"
RM_RF  = @if exist "$(1)" rmdir /s /q "$(1)"
# Find makensis.exe — check PATH first, then common install locations
MAKENSIS := $(shell where makensis 2>NUL || echo "C:\Program Files (x86)\NSIS\makensis.exe")
else
MKDIR_P = @mkdir -p $(1)
RM_RF  = @rm -rf $(1)
MAKENSIS := $(shell command -v makensis 2>/dev/null || true)
endif

## release-dev: quick build for current platform with dev version tag
release-dev: frontend/dist
	$(call MKDIR_P,$(DIST_DIR))
ifeq ($(OS),Windows_NT)
	wails build -clean -o heka-gui$(EXE) -ldflags "$(LDFLAGS)"
	wails build -windowsconsole -o heka$(EXE) -ldflags "$(LDFLAGS)"
else
	wails build -clean -nopackage -ldflags "$(LDFLAGS)"
endif
	powershell -NoProfile -Command "Copy-Item build\bin\heka$(EXE) $(DIST_DIR)\heka$(EXE) -Force"
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -Command "Copy-Item build\bin\heka-gui$(EXE) $(DIST_DIR)\heka-gui$(EXE) -Force"
endif
	@echo.
	@echo   Dev build v$(VERSION) ^> $(DIST_DIR)/

## release: full multi-platform build with installers
release: release-windows release-linux release-mac
	@echo.
	@echo   All platforms built ^> $(DIST_DIR)/
	@echo.

## release-windows: NSIS installer containing console and GUI executables
release-windows: frontend/dist
	@echo ^> Windows...
	$(call MKDIR_P,$(DIST_DIR))
	wails build -clean -o heka-gui$(EXE) -ldflags "$(LDFLAGS)"
	wails build -windowsconsole -o heka$(EXE) -ldflags "$(LDFLAGS)"
	cd build\windows\installer && set "PATH=C:\Program Files (x86)\NSIS;$(PATH)" && makensis -DINFO_PRODUCTVERSION=$(VERSION) -DARG_WAILS_AMD64_BINARY=..\..\bin\heka-gui.exe -DARG_HEKA_AMD64_BINARY=..\..\bin\heka.exe project.nsi
	powershell -NoProfile -Command "Copy-Item build\bin\Heka-amd64-installer.exe $(DIST_DIR)\heka-$(VERSION)-amd64-setup.exe -Force"
	@echo   OK $(DIST_DIR)/heka-$(VERSION)-amd64-setup.exe

## release-linux: binary + .deb package (cross-compiled from Windows)
release-linux: frontend/dist
	@echo ^> Linux (deb^)...
	$(call MKDIR_P,$(DIST_DIR)/linux-deb/usr/bin)
	$(call MKDIR_P,$(DIST_DIR)/linux-deb/DEBIAN)
	wails build -clean -platform linux/amd64 -nopackage -ldflags "$(LDFLAGS)"
	powershell -NoProfile -Command "Copy-Item build\bin\heka $(DIST_DIR)\linux-deb\usr\bin\heka -Force"
	@echo Package: heka                                         >  $(DIST_DIR)/linux-deb/DEBIAN/control
	@echo Version: $(VERSION)                                  >> $(DIST_DIR)/linux-deb/DEBIAN/control
	@echo Section: utils                                       >> $(DIST_DIR)/linux-deb/DEBIAN/control
	@echo Priority: optional                                   >> $(DIST_DIR)/linux-deb/DEBIAN/control
	@echo Architecture: amd64                                  >> $(DIST_DIR)/linux-deb/DEBIAN/control
	@echo Depends: webkit2gtk-4.0                              >> $(DIST_DIR)/linux-deb/DEBIAN/control
	@echo "Maintainer: Azizul Hakim <mdazizulhakim.cse@gmail.com>" >> $(DIST_DIR)/linux-deb/DEBIAN/control
	@echo Description: A local task runner and scheduler       >> $(DIST_DIR)/linux-deb/DEBIAN/control
	@echo  Heka is a desktop app for running and scheduling    >> $(DIST_DIR)/linux-deb/DEBIAN/control
	@echo  tasks with a system tray, CLI, and GUI.             >> $(DIST_DIR)/linux-deb/DEBIAN/control
	dpkg-deb --build $(DIST_DIR)/linux-deb $(DIST_DIR)/heka-$(VERSION)-amd64.deb
	$(call RM_RF,$(DIST_DIR)/linux-deb)
	@echo   OK $(DIST_DIR)/heka-$(VERSION)-amd64.deb

## release-mac: .app bundle (must be built on macOS or a CI mac runner)
release-mac: frontend/dist
	@echo ^> macOS (.app^)...
	$(call MKDIR_P,$(DIST_DIR))
	wails build -clean -platform darwin/universal -ldflags "$(LDFLAGS)"
	@echo   OK build/bin/heka.app

# main.go embeds all:frontend/dist, so it must exist for go build/test.
frontend/dist:
	cd frontend && npm run build