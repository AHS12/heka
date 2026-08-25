# Heka build tooling — single entry point for dev, build, and checks (SPEC-01 §4).

BIN_DIR := build

.PHONY: dev dev-core build test check vet lint format clean

ifeq ($(OS),Windows_NT)
EXE := .exe
else
EXE :=
endif

BIN     := $(BIN_DIR)/bin/heka$(EXE)
GUI_BIN := $(BIN_DIR)/bin/heka-gui$(EXE)

## dev: run the daemon and GUI together for development (Ctrl-C stops both)
dev:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev.ps1

## dev-core: run only the daemon in the foreground (own terminal)
dev-core: frontend/dist
	go run . daemon

## build: build the console binary; on Windows also the GUI-subsystem flavor
build:
	wails build
ifeq ($(OS),Windows_NT)
	wails build -o heka-gui$(EXE) -ldflags "-H windowsgui"
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

# main.go embeds all:frontend/dist, so it must exist for go build/test.
frontend/dist:
	cd frontend && npm run build