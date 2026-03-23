.PHONY: run-daemon run-ui build build-legacy clean

BINARY_NAME ?= archon
BUILD_DIR ?= dist
BUILD_PATH ?= ./cmd/archon
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -X main.appVersion=$(VERSION) -X main.appCommit=$(COMMIT) -X main.appBuildDate=$(BUILD_DATE)

run-daemon:
	@go run ./cmd/archon daemon

run-ui:
	@echo "UI not implemented yet (Phase 3)."

build:
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_PATH)

build-legacy:
	@go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) $(BUILD_PATH)

clean:
	@rm -rf $(BUILD_DIR)
