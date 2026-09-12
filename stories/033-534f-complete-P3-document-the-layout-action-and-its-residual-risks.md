---
id: 033-534f
title: Document the layout action and its residual risks
status: complete
priority: P3
type: chore
created: "2026-09-10T00:58:00.969Z"
updated: "2026-09-12T02:53:55.550Z"
dependencies: ["027", "029", "031"]
plan: plans/layout-authoring-editor.md
plan_step: Step 9
depends_on: ["stories/027-7e5e-pending-P1-host-only-layout-action-handler-reachable-from-bot.md", "stories/029-f12a-pending-P2-editor-drag-and-resize-with-collision-rejection.md", "stories/031-a610-pending-P2-config-panel-drawn-from-field-descriptors.md"]
started_at: "2026-09-12T02:35:49.829Z"
completed_at: "2026-09-12T02:53:55.550Z"
---

# Document the layout action and its residual risks

## Problem Statement

The layout action is a new client-writable protocol surface with an accepted residual risk (host-supplied media URIs and Lua source reach every player). That risk must be findable by someone about to deploy, not buried in a plan file.

## Acceptance Criteria

- [x] VERIFY: go build ./...
- [x] docs/PROTOCOL.md gains a layout section under Client to server with the full op table and the authorization rules, matching the existing per-action format
- [x] docs/ARCHITECTURE.md gains a layout-engine section
- [x] CLAUDE.md records that game.data.layout is the canonical layout location, that privateData is reserved anywhere inside it, and that the Go and TypeScript validators are a deliberate duplicated pair that must change together
- [x] The accepted residual risk (host-supplied URIs and script source delivered to and executed by every player) is documented where a deployer will find it
- [x] The payload and widget-count caps are documented with their actual values

## Files

- docs/PROTOCOL.md
- docs/ARCHITECTURE.md
- CLAUDE.md

## Proof

- [x] [completeness] Completeness (All 6 criteria checked; four discrepancies between the plan and the shipped code were found and documented as shipped rather than as planned.)
- [x] [feature-availability] Feature availability (Documents the action as reachable over all three transports and explicitly notes refresh's missing GraphQL mutation so nobody assumes parity.)
- [x] [robustness] Robustness (Every constant was read from validate.go rather than copied from the plan, and the plan was wrong in four places that are now documented as shipped.)
- [~] [resilience] Resilience (Documentation only; no runtime behaviour.)
- [x] [security] Security (The accepted residual risk - host-supplied media URIs and Lua source delivered to and executed by every player, with no allow-listing or review - is a TOP-LEVEL section in ARCHITECTURE.md opening with 'Read this before any public deployment', not buried in a subsection, and is cross-linked from CLAUDE.md.)
- [x] [defense-in-depth] Defense in depth (Records that the Go validator is the security boundary and rejects, while the TS one is UX feedback and clamps - and that a rule enforced only on the client is not enforced.)
- [x] [input-validation] Input validation (Documents that unknown keys are rejected PER OP, so a field valid for one op is an error on another, and that board scope must carry neither id.)
- [~] [thread-safety] Thread safety (Documentation only.)
- [~] [configurability] Configurability (Documentation only.)

## QA

go build clean. Constants spot-checked against validate.go. Four plan/code discrepancies found and documented as shipped, including one where my own task brief was wrong about the failure UI.

## Work Log

### 2026-09-12T02:53:54.054Z - PROTOCOL.md gains a layout section between kick and logout in the existing per-action format: example message, the seven-op table, the scope table, per-op unknown-key rejection, 'there is no reply - the published delta is the response', the limits table, and a deployment warning linking to ARCHITECTURE.md. Plus a layout row in the GraphQL mutation table (noting code and op are set last so args cannot redirect the edit) and POST /api/layout in the REST table, with a carve-out on the existing 'unknown body keys are dropped' sentence since layout is the one route passing the body through whole. ARCHITECTURE.md gains a Layout engine section and, as its own TOP-LEVEL section rather than nested, 'Host-authored content - accepted risk', leading with 'Read this before any public deployment'. CLAUDE.md gains a terse Layout subsection. FOUR PLACES THE PLAN DISAGREED WITH THE SHIPPED CODE, all documented as shipped: (1) the plan asks for a script-length cap; none shipped - validate.go only type-checks script as a string and maxBytes bounds it transitively, so a single script can be almost 256KiB of text every client executes. Documented as an explicit gap. (2) refresh has no GraphQL mutation - verified against schema.graphqls - so an explicit note says not to assume parity. (3) MY OWN BRIEF WAS WRONG: I told the agent a rejected layout 'renders as a blank board'. board-view.tsx renders 'This board could not be loaded.' via BoardMessage; I verified that myself afterwards. The accurate behaviour is documented - one typo takes the board down for everyone, with a fallback message. (4) setGrid is NOT scoped: it takes only grid and always targets the board grid; a sub-grid's grid lives in its config and is changed with setWidgetConfig. And setScript uses source, a *string, so empty deletes the field. Constants were read from validate.go, not guessed: maxWidgets 300, maxBytes 256<<10, maxDepth 4 (four grid levels counting the scene's own, so at most three nested sub-grids), minDim/maxDim 8/4096 - I spot-checked all of these against the source.


### 2026-09-12T02:53:54.580Z - Proof completeness set PROVEN: All 6 criteria checked. go build clean; constants independently spot-checked against validate.go.

### 2026-09-12T02:53:54.659Z - Proof security set PROVEN: The accepted residual risk - host-supplied media URIs and Lua source delivered to and executed by every player, with no allow-listing or review - is a TOP-LEVEL section in ARCHITECTURE.md opening with 'Read this before any public deployment', not buried in a subsection, and is cross-linked from CLAUDE.md.

### 2026-09-12T02:53:54.744Z - Proof completeness set PROVEN: All 6 criteria checked; four discrepancies between the plan and the shipped code were found and documented as shipped rather than as planned.

### 2026-09-12T02:53:54.830Z - Proof input-validation set PROVEN: Documents that unknown keys are rejected PER OP, so a field valid for one op is an error on another, and that board scope must carry neither id.

### 2026-09-12T02:53:54.908Z - Proof defense-in-depth set PROVEN: Records that the Go validator is the security boundary and rejects, while the TS one is UX feedback and clamps - and that a rule enforced only on the client is not enforced.

### 2026-09-12T02:53:54.992Z - Proof robustness set PROVEN: Every constant was read from validate.go rather than copied from the plan, and the plan was wrong in four places that are now documented as shipped.

### 2026-09-12T02:53:55.069Z - Proof feature-availability set PROVEN: Documents the action as reachable over all three transports and explicitly notes refresh's missing GraphQL mutation so nobody assumes parity.

### 2026-09-12T02:53:55.146Z - Proof resilience set NOT_APPLICABLE: Documentation only; no runtime behaviour.

### 2026-09-12T02:53:55.220Z - Proof thread-safety set NOT_APPLICABLE: Documentation only.

### 2026-09-12T02:53:55.293Z - Proof configurability set NOT_APPLICABLE: Documentation only.
