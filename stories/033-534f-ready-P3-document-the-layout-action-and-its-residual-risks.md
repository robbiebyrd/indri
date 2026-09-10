---
id: 033-534f
title: Document the layout action and its residual risks
status: ready
priority: P3
type: chore
created: "2026-09-10T00:58:00.969Z"
updated: "2026-09-10T00:58:09.426Z"
dependencies: ["027", "029", "031"]
plan: plans/layout-authoring-editor.md
plan_step: Step 9
depends_on: ["stories/027-7e5e-pending-P1-host-only-layout-action-handler-reachable-from-bot.md", "stories/029-f12a-pending-P2-editor-drag-and-resize-with-collision-rejection.md", "stories/031-a610-pending-P2-config-panel-drawn-from-field-descriptors.md"]
---

# Document the layout action and its residual risks

## Problem Statement

The layout action is a new client-writable protocol surface with an accepted residual risk (host-supplied media URIs and Lua source reach every player). That risk must be findable by someone about to deploy, not buried in a plan file.

## Acceptance Criteria

- [ ] VERIFY: go build ./...
- [ ] docs/PROTOCOL.md gains a layout section under Client to server with the full op table and the authorization rules, matching the existing per-action format
- [ ] docs/ARCHITECTURE.md gains a layout-engine section
- [ ] CLAUDE.md records that game.data.layout is the canonical layout location, that privateData is reserved anywhere inside it, and that the Go and TypeScript validators are a deliberate duplicated pair that must change together
- [ ] The accepted residual risk (host-supplied URIs and script source delivered to and executed by every player) is documented where a deployer will find it
- [ ] The payload and widget-count caps are documented with their actual values

## Files

- docs/PROTOCOL.md
- docs/ARCHITECTURE.md
- CLAUDE.md

## Proof

- [ ] [completeness] Completeness
- [ ] [feature-availability] Feature availability
- [ ] [robustness] Robustness
- [ ] [resilience] Resilience
- [ ] [security] Security
- [ ] [defense-in-depth] Defense in depth
- [ ] [input-validation] Input validation
- [ ] [thread-safety] Thread safety
- [ ] [configurability] Configurability

## Work Log

