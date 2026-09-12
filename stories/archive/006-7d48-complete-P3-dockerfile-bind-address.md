---
id: 006-7d48
status: complete
priority: P3
type: fix
created: "2026-09-09T18:58:45Z"
source_review: reviews/review-2026-09-09-fixes.md
source_finding: OPS-1
completed_at: "2026-09-09T19:07:07.900Z"
updated: "2026-09-09T19:07:07.901Z"
---
# Make the container bind a reachable address

## Description
`INDRI_LISTEN_ADDRESS` defaults to `localhost`; the server binds `ListenAddress + ":" + port`, so
inside a container it binds loopback only and Docker port-forwarding to the published port never
reaches it — `EXPOSE 5002` is misleading without `INDRI_LISTEN_ADDRESS=0.0.0.0`.

## Acceptance Criteria
- The image serves on the published port without the operator overriding the bind address (e.g. `ENV INDRI_LISTEN_ADDRESS=0.0.0.0`).
- Document the expectation if `localhost` stays the default for local runs.

## Work Log

### 2026-09-09T19:07:07.823Z - Dockerfile: ENV INDRI_LISTEN_ADDRESS=0.0.0.0 so the container is reachable via the published port.

