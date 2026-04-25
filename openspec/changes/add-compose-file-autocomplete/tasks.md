## 1. Compose Autocomplete Contract

- [x] 1.1 Audit compose `@` handling in `internal/app` so supported-provider and unsupported-provider behavior matches the new spec.
- [x] 1.2 Add or tighten UI/controller tests for picker activation, unsupported-provider fallback, and plain-text mention insertion.

## 2. File Search Transport Contract

- [x] 2.1 Audit the typed client and daemon handlers for `POST`, `PATCH`, `DELETE`, and SSE follow routes under `/v1/file-searches`.
- [x] 2.2 Add or tighten tests for unsupported-provider errors, invalid JSON/method handling, and streaming result events.

## 3. Documentation And Verification

- [x] 3.1 Update `README.md` and `docs/architecture.md` so compose autocomplete docs match the locked contract.
- [x] 3.2 Run focused tests for `internal/app`, `internal/client`, and `internal/daemon` file-search coverage.
- [x] 3.3 Run `openspec validate add-compose-file-autocomplete`.
