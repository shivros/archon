// Package acp is a reusable client for the Agent Client Protocol.
//
// ACP is a JSON-RPC 2.0 protocol used to connect editors/clients to coding
// agents over stdio. See https://agentclientprotocol.com/ for the spec.
//
// The package exposes a Client that manages the agent subprocess, frames
// messages as newline-delimited JSON, correlates requests to responses,
// fans notifications out to subscribers, and dispatches incoming
// agent-to-client requests to registered handlers. It knows nothing about
// any specific agent — provider adapters (e.g. Hermes) are built on top.
package acp
