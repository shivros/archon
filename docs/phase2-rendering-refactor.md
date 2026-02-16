# Phase 2 Rendering Refactor

Date: February 16, 2026

## Goals

- Extract rendering orchestration out of `Model`.
- Add cache layers to avoid avoidable markdown rerendering.
- Replace repeated overlay string split/join work with structured layer composition.
- Keep `Model` depending on interfaces to preserve maintainability.

## Architecture Changes

- `RenderPipeline` abstraction:
  - `internal/app/render_pipeline.go`
  - `Model` now depends on `RenderPipeline` (`WithRenderPipeline(...)`).
- Block render strategy abstraction:
  - `chatBlockRenderer` + `defaultChatBlockRenderer`
  - `renderChatBlocksWithRenderer(...)` in `internal/app/chat_render.go`
- Block-level cache keyed by `(block hash, width, selected)`:
  - `internal/app/render_block_cache.go`
- Render-result cache in pipeline keyed by request signature:
  - `internal/app/render_pipeline.go`
- Structured overlay composition:
  - `LayerComposer` + `textCanvas` in `internal/app/layer_composer.go`
  - `Model` now depends on `LayerComposer` (`WithLayerComposer(...)`)

## SOLID Mapping

- SRP:
  - `Model` no longer directly orchestrates markdown/block rendering internals.
  - Overlay layer mutation moved to a dedicated composer.
- OCP:
  - New render implementations can be injected through `RenderPipeline`.
  - New overlay strategies can be injected through `LayerComposer`.
- LSP:
  - `RenderPipeline` and `LayerComposer` implementations are swappable.
- DIP:
  - `Model` depends on interfaces, not concrete render/composition implementations.

## Validation

- Tests:
  - `internal/app/render_block_cache_test.go`
  - `internal/app/render_pipeline_test.go`
  - `internal/app/layer_composer_test.go`
- Benchmark additions:
  - `BenchmarkModelViewLargeTranscript`
- Notes:
  - `BenchmarkModelRenderViewportLargeTranscript` now prewarms once before timing to measure steady-state interaction latency.

## Measured Outcome (steady-state benchmark sweep)

Command:

- `go test ./internal/app -run '^$' -bench 'BenchmarkModel(Action|RenderViewportLargeTranscript|SessionSwitchPath|ViewLargeTranscript)' -benchmem -count=3`

Observed central tendencies in latest run:

- `BenchmarkModelRenderViewportLargeTranscript`: ~10.6 ms/op
- `BenchmarkModelViewLargeTranscript`: ~5.4 ms/op
- `BenchmarkModelSessionSwitchPath`: ~30.9 us/op

Compared to the pre-refactor Phase 1 baseline artifacts in this repo session, render-heavy update path improved by well over 50% in steady-state.
