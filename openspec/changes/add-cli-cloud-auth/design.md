## Context

The cloud-auth commands already exist and are test-covered, but their contract is spread across code, README prose, and daemon behavior. The CLI experience is intentionally human-oriented rather than JSON-oriented, so the spec needs to preserve that presentation contract without overfitting to internal implementation details.

## Goals / Non-Goals

**Goals:**
- Specify the device-flow contract for `login`.
- Specify the human-readable status contract for `whoami`.
- Specify the success-message contract for `logout`.

**Non-Goals:**
- Introducing machine-readable output modes.
- Changing the persistence model for cloud credentials.
- Reworking the browser/device-flow protocol itself.

## Decisions

- Keep all three commands under one capability because they represent one user-facing workflow: link, inspect, unlink.
- Treat the fallback `Visit:` URL and `Code:` output from `login` as mandatory, even when a browser open is attempted successfully.
- Preserve the current resilience strategy:
  - `--no-browser` suppresses any opener attempt.
  - browser-open failures warn on stderr but do not fail the login flow.
  - `slow_down` increases the poll interval rather than aborting.
- Keep `whoami` human-readable. The command is intended for terminal use and quick diagnostics, not as a machine API.

## Risks / Trade-offs

- **Human-readable output is harder for automation to parse** -> That is acceptable for this workflow; if scripts need machine output later, a separate deliberate change can add it.
- **Polling semantics depend on daemon/cloud state transitions** -> The spec defines the user-visible behavior and leaves room to change the underlying transport as long as the same status handling remains intact.
- **Logout can partially succeed** -> The spec explicitly captures the daemon-provided message so users are told whether only local credentials were cleared.
