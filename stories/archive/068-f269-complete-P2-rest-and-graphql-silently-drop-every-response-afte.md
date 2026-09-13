---
id: 068-f269
title: REST and GraphQL silently drop every response after the first
status: complete
priority: P2
type: fix
created: "2026-09-13T18:49:24.478Z"
updated: "2026-09-13T23:13:41.320Z"
dependencies: []
completed_at: "2026-09-13T23:13:41.320Z"
---

# REST and GraphQL silently drop every response after the first

## Problem Statement

router.Dispatch merges results from every handler matching an action (internal/handlers/router/act.go:46 appends Responses), and registration is additive — a game handler registered alongside a built-in, or a received/processed hook, produces more than one response. WebSocket writes them all (internal/services/boot/handlers.go:157), but REST (internal/transport/rest/rest.go:164) and GraphQL (internal/transport/graphql/resolvers/resolver.go:79) both take Responses[0] and discard the rest with no error and no log. Today every built-in returns exactly one response so nothing hits it, but story 061 (Lua hooks on built-in actions) makes multi-handler dispatch routine, and story 054 caps indri.reply at one call specifically to dodge this — which protects scripts without fixing the underlying divergence. Aggregating into an array would change the REST and GraphQL wire contract, so this needs a deliberate decision rather than a quiet fix inside another story.

## Acceptance Criteria

- [x] A decision is recorded on whether REST and GraphQL aggregate, return only the first with an explicit contract, or error when a dispatch produces more than one response
- [x] Whatever is chosen, the behaviour is identical across REST, GraphQL and WebSocket, or the divergence is documented in docs/PROTOCOL.md as intentional
- [x] A test asserts the chosen behaviour on all three transports for a two-handler action
- [REJECTED] If the wire contract changes, client/services is updated to match and docs/PROTOCOL.md records the new shape (Antecedent is false: the wire contract was deliberately not changed. Option (b) was chosen - keep one document per request-response reply and report the drop - so client/services needed no update. docs/PROTOCOL.md does record the per-transport behaviour, which is the part of this criterion that applied.)
- [x] VERIFY: go test -race ./internal/transport/... ./internal/handlers/router/

## Proof

- [x] [completeness] Completeness (4 of 5 criteria met and the fifth rejected as inapplicable; decision recorded, behaviour documented, all three transports tested)
- [x] [feature-availability] Feature availability (FirstResponse is on the two request-response transports; the stream transports are pinned by a test asserting handleClientMessage still writes both frames)
- [x] [robustness] Robustness (A hard error on more than one response was rejected because it would break reconnect-into-a-game over REST and GraphQL, turning a degraded result into an outright failure)
- [x] [resilience] Resilience (Single-response behaviour, which is every other action, is byte-identical and logs nothing)
- [~] [security] Security (No authorisation or input surface; the change is observability on an existing reply path)
- [x] [defense-in-depth] Defense in depth (The boot test pins the stream side, so a future unification that truncates WebSocket or WebRTC fails loudly instead of silently)
- [~] [input-validation] Input validation (No external input)
- [~] [thread-safety] Thread safety (Stateless helper over per-request values)
- [~] [configurability] Configurability (No tunable; the per-transport shape is documented in PROTOCOL.md instead)

## QA

Verified independently: transport, rest, graphql and boot packages green under -race. CORRECTED A FALSE PREMISE IN MY OWN STORY - I wrote that the bug was latent; it is live. reconnect/handler.go:56-72 appends a second response when the session has a GameID, and reconnect is exposed at POST /api/reconnect and as the reconnect GraphQL mutation, so a REST or GraphQL client resuming into an active game loses its keyframe today. Confirmed by reading the handler, routes.go:23 and schema.graphqls:17, and that no other handler returns more than one.

## Work Log

### 2026-09-13T23:12:31.397Z - Investigated, decided and implemented. Correction to the problem statement: the bug is NOT latent. internal/handlers/actions/reconnect/handler.go:56-72 returns TWO responses when the resumed session has a GameID (auth payload, then keyframe), and reconnect is exposed as POST /api/reconnect and the reconnect GraphQL mutation. So REST and GraphQL callers resuming into a game lose the keyframe today, silently. No other registered handler (internal/handlers/actions/*, example/tictactoe) returns more than one.

Client evidence: the reference client has no REST or GraphQL consumer at all. The only fetch() in client/ is the WebRTC signalling POST in services/webrtc-transport.ts; MessageHandler is fed solely by ClientTransport (WebSocket/WebRTC), and GameStateParser is instantiated only by message-handler.ts. A response-shape change on REST/GraphQL would cost the client nothing -- and gain it nothing.

Decision: option (b) -- keep one response per request, make the drop loud, and document the divergence as intentional. Rejected (a) aggregate-to-array: it changes a public contract (GraphQL JSON! -> [JSON!]!, REST object -> array) and taxes every caller of every action with an unwrap for a case that arises in one action. Rejected hard-erroring on >1: that would break reconnect-into-a-game over REST/GraphQL, a live and documented path. Responses is a message-stream concept; WebSocket and WebRTC are streams, a REST request and a GraphQL mutation are not. The divergence is inherent to the transports -- the defect was the silence.

Implemented (no wire format change): internal/transport/response.go adds FirstResponse(action, responses), which returns the first document and log.Printf's the action and the number dropped when there is more than one. internal/transport/rest/rest.go writeJSON and internal/transport/graphql/resolvers/resolver.go dispatch both route through it; single-response behaviour (every action today) is byte-identical and silent.

Tests (stdlib, table-driven, no DB, no skips): internal/transport/response_test.go covers zero/one/many plus the report content; internal/transport/rest/rest_test.go and internal/transport/graphql/graphql_test.go drive real httptest servers with two handlers registered on the logout action through the real router and assert the first document plus the report; internal/services/boot/handlers_test.go asserts handleClientMessage writes BOTH frames, pinning the stream side of the divergence. Each also asserts the single-response case is unchanged and logs nothing.

docs/PROTOCOL.md: new section 'How many responses an action produces' with the per-transport table, cross-referenced from the reconnect action, the GraphQL mutations section, the REST responses section, and the SSE bootstrapping recipe.

Verified: go build ./... && go vet ./... && go test -race ./... all green; go test -race -count=1 ./internal/transport/... ./internal/handlers/router/ green. Client untouched. Criterion 4 left unchecked: its antecedent ('if the wire contract changes') is false -- the contract did not change, so client/services needed no update.


### 2026-09-13T23:13:30.670Z - Proof completeness set PROVEN: 4 of 5 criteria met and the fifth rejected as inapplicable; decision recorded, behaviour documented, all three transports tested

### 2026-09-13T23:13:30.748Z - Proof feature-availability set PROVEN: FirstResponse is on the two request-response transports; the stream transports are pinned by a test asserting handleClientMessage still writes both frames

### 2026-09-13T23:13:30.883Z - Proof robustness set PROVEN: A hard error on more than one response was rejected because it would break reconnect-into-a-game over REST and GraphQL, turning a degraded result into an outright failure

### 2026-09-13T23:13:31.121Z - Proof resilience set PROVEN: Single-response behaviour, which is every other action, is byte-identical and logs nothing

### 2026-09-13T23:13:31.212Z - Proof security set NOT_APPLICABLE: No authorisation or input surface; the change is observability on an existing reply path

### 2026-09-13T23:13:31.304Z - Proof defense-in-depth set PROVEN: The boot test pins the stream side, so a future unification that truncates WebSocket or WebRTC fails loudly instead of silently

### 2026-09-13T23:13:31.395Z - Proof input-validation set NOT_APPLICABLE: No external input

### 2026-09-13T23:13:31.489Z - Proof thread-safety set NOT_APPLICABLE: Stateless helper over per-request values

### 2026-09-13T23:13:31.595Z - Proof configurability set NOT_APPLICABLE: No tunable; the per-transport shape is documented in PROTOCOL.md instead
