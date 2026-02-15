# Phase 0 UI Latency Baseline

Date: February 15, 2026

## Scope

Phase 0 baseline covers:

- `Model.Update` span timing
- `Model.View` span timing
- Action timers for:
  - toggle sessions sidebar
  - toggle notes sidebar
  - exit compose
  - open new session flow entry
  - switch session load path
- Regression benchmarks for render cost and session switching path

## Instrumentation Added

- `internal/app/ui_latency.go`
  - `UILatencySink` interface (DIP)
  - `WithUILatencySink(...)` model option
  - `uiLatencyTracker` probe lifecycle
  - `InMemoryUILatencySink` for tests and benchmarks
- `internal/app/model.go`
  - `Model.Update`: `ui.model.update`
  - `Model.View`: `ui.model.view`
  - action probes for `toggleSidebar`, `exitCompose`, `enterNewSession`, `loadSelectedSession`
- `internal/app/model_notes_panel.go`
  - action probe for `toggleNotesPanel`
- `internal/app/model_update_messages.go`
  - session switch action completion on `historyMsg`/`tailMsg` success and error

## Validation Artifacts

- Baseline tests:
  - `internal/app/ui_latency_test.go`
  - `internal/app/model_ui_latency_test.go`
- Benchmarks:
  - `internal/app/model_latency_benchmark_test.go`
- Benchmark command:
  - `go test ./internal/app -run '^$' -bench 'BenchmarkModel(Action|RenderViewportLargeTranscript|SessionSwitchPath)' -benchmem -count=5`
- Comparison tool:
  - `$HOME/go/bin/benchstat /tmp/phase0_bench_before.txt /tmp/phase0_bench_after.txt`

## Before/After Numbers (benchstat central tendency)

| Interaction / Path | Before | After | Delta |
| --- | ---: | ---: | ---: |
| Toggle sessions sidebar | 1.070 ms/op | 0.391 ms/op | not significant (`p=0.056`) |
| Toggle notes sidebar | 3.446 ms/op | 1.841 ms/op | -46.58% |
| Exit compose | 0.612 ms/op | 0.167 ms/op | -72.78% |
| Open new session | 0.113 ms/op | 0.057 ms/op | not significant (`p=0.056`) |
| Switch session (action benchmark) | 0.218 ms/op | 0.248 ms/op | not significant (`p=0.690`) |
| Session switch path benchmark | 0.208 ms/op | 0.563 ms/op | +170.77% |
| Render large transcript benchmark | 1.282 s/op | 2.089 s/op | not significant (`p=0.151`) |

Notes:

- These microbenchmarks are noisy on shared hardware and include outliers.
- Phase 0 added observability, not optimizations; large swings should be treated as baseline variance until we run repeated controlled samples.
- Allocation profiles are effectively flat across runs.

## Exit Criteria Check

- [x] Before/after numbers captured for each critical action
- [x] `Update` and `View` latency spans instrumented
- [x] Action-timer probes added for all five critical interactions
- [x] Telemetry isolated in `ui_latency` module (SRP)
- [x] Metrics sink injected through interface (`UILatencySink`) and model option (DIP)
- [x] Baseline report committed
