## ADDED Requirements

### Requirement: The daemon SHALL expose a provider-agnostic file-search create endpoint
Archon SHALL expose `POST /v1/file-searches` for compose file-search creation. The request SHALL identify the target provider through the search scope and SHALL return a newly created file-search session document when accepted.

#### Scenario: Successful create returns a file-search session
- **WHEN** a caller posts a valid JSON body to `/v1/file-searches` containing a provider scope and query
- **THEN** the daemon MUST create a file-search session
- **AND** the daemon MUST return HTTP `201`
- **AND** the response body MUST describe the created file-search session, including its id, provider, query, and status

#### Scenario: Unsupported provider returns a structured unsupported error
- **WHEN** a caller posts a file-search create request for a provider that does not support file search
- **THEN** the daemon MUST return HTTP `400`
- **AND** the response body MUST include the error code `file_search_unsupported`

### Requirement: The daemon SHALL support updating and closing file-search sessions
Archon SHALL expose `PATCH /v1/file-searches/{id}` to update a file-search session and `DELETE /v1/file-searches/{id}` to close one.

#### Scenario: PATCH updates query and limit
- **WHEN** a caller sends `PATCH /v1/file-searches/fs-1` with a valid JSON body containing a new query and limit
- **THEN** the daemon MUST apply those updated search parameters to the existing session
- **AND** the daemon MUST return HTTP `200` with the updated file-search session document

#### Scenario: DELETE closes the search session
- **WHEN** a caller sends `DELETE /v1/file-searches/fs-1`
- **THEN** the daemon MUST close that search session
- **AND** the daemon MUST return HTTP `200`

### Requirement: The daemon SHALL expose a follow-only SSE stream for file-search events
Archon SHALL expose `GET /v1/file-searches/{id}/events?follow=1` as a server-sent-events stream for file-search updates. The stream SHALL carry normalized file-search events until the session completes or is closed.

#### Scenario: Follow stream returns SSE events
- **WHEN** a caller performs `GET /v1/file-searches/fs-1/events?follow=1`
- **THEN** the daemon MUST return HTTP `200`
- **AND** the response `Content-Type` MUST be `text/event-stream`
- **AND** the stream MUST emit normalized file-search events for that search id

#### Scenario: Missing `follow=1` is rejected
- **WHEN** a caller performs `GET /v1/file-searches/fs-1/events` without `follow=1`
- **THEN** the daemon MUST reject the request with HTTP `400`

### Requirement: File-search endpoints SHALL reject malformed JSON and unsupported methods cleanly
File-search transport errors SHALL fail with standard HTTP errors rather than partial success or hanging streams.

#### Scenario: Malformed create or update JSON is rejected
- **WHEN** a caller sends malformed JSON to `POST /v1/file-searches` or `PATCH /v1/file-searches/{id}`
- **THEN** the daemon MUST return HTTP `400`

#### Scenario: Unsupported methods and unknown subpaths are rejected
- **WHEN** a caller uses an unsupported method or unknown subpath under `/v1/file-searches`
- **THEN** the daemon MUST return the appropriate HTTP error such as `405` for wrong method or `404` for unknown route
