.PHONY: run-daemon run-ui build

run-daemon:
	@go run ./cmd/control daemon

run-ui:
	@echo "UI not implemented yet (Phase 3)."

build:
	@go build ./cmd/control
