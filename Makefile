.PHONY: run-daemon run-ui build

run-daemon:
	@go run ./cmd/archon daemon

run-ui:
	@echo "UI not implemented yet (Phase 3)."

build:
	@go build ./cmd/archon
