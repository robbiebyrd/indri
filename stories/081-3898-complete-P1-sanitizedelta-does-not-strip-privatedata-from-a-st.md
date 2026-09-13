---
id: 081-3898
title: SanitizeDelta does not strip privateData from a struct value
status: complete
priority: P1
type: fix
created: "2026-09-13T23:40:46.695Z"
updated: "2026-09-13T23:53:54.070Z"
dependencies: []
completed_at: "2026-09-13T23:53:54.070Z"
---

# SanitizeDelta does not strip privateData from a struct value

## Problem Statement

events.stripKey switches on map[string]interface{} and []interface{} and returns everything else untouched, so a struct value passes through unstripped. core.UpdateField publishes its value verbatim as map[string]interface{}{key: value}, so any caller passing a struct that contains PrivateData broadcasts it to every player in the game. This is currently unreachable only because every UpdateField caller lives in internal/services/stage, which nothing imports - and two of those callers already pass structs that carry private data: stage.go:197 passes the whole models.Stage and stage.go:133 passes a models.Scene. UpdateField is on the exported game Storer interface and CLAUDE.md tells game authors to use it for single-field sets, so the next correct-looking caller leaks. CLAUDE.md also states that GameService.Sanitize and events.SanitizeDelta must agree on what is hidden; on this path they do not.

## Acceptance Criteria

- [x] A struct value published through UpdateField has privateData stripped from the delta, at every depth, including nested structs and pointers to structs
- [x] A regression test proves the leak using a real models.Stage carrying private data at both stage and scene level, and fails if stripKey stops descending into structs
- [x] Sanitize and SanitizeDelta agree for struct values as well as map values, which CLAUDE.md requires
- [x] The fix does not change what is stored, only what is published
- [x] VERIFY: go test -race ./internal/services/events/ ./internal/repo/game/

## Files

- internal/services/events/delta.go

## Proof

- [x] [completeness] Completeness (5 of 5 criteria; structs, pointers, maps and slices of structs, nested and embedded fields all covered)
- [x] [feature-availability] Feature availability (The end-to-end test exercises UpdateField on both the memory and mongodb backends, so the real publish path is covered rather than only the helper)
- [x] [robustness] Robustness (RED proved by restoring the pre-fix delta.go from HEAD: all ten table cases failed, publishing the stage and scene private stores verbatim, and the store contract test failed on both backends)
- [x] [resilience] Resilience (SanitizeDelta now fails closed - a value that cannot be rendered as JSON cannot be shown to be private-free, so that path is dropped and logged rather than broadcast unexamined, and loses nothing because such a value could never have reached a client anyway)
- [x] [security] Security (Closes a P1 sanitization bypass: UpdateField published its value verbatim and stripKey returned any non-map non-slice untouched, so a struct carrying PrivateData reached every player; two existing stage callers already pass such structs)
- [x] [defense-in-depth] Defense in depth (A test asserts the delta agrees with GameService.Sanitize compared through the JSON view on both sides, which also catches over-stripping, and CLAUDE.md requires those two to agree)
- [x] [input-validation] Input validation (Value preservation is pinned including MaxInt64 via UseNumber, byte slices, and embedded, renamed, omitted and hidden fields, so only the Go type and key order change and never the wire bytes)
- [x] [thread-safety] Thread safety (A test asserts the caller's own value is not mutated; suite green under -race)
- [~] [configurability] Configurability (No configuration; the rule is fixed by what the client is allowed to see)

## QA

Verified independently: build, vet, gofmt and full suite green under -race, zero skips. Confirmed stripKey now returns an error and SanitizeDelta fails closed, and that UseNumber preserves number literals. RED was proved against the pre-fix code restored from HEAD, on both the events unit path and the end-to-end store contract on both backends. 5/5.

## Work Log

### 2026-09-13T23:51:46.646Z - Fixed the struct bypass in events.SanitizeDelta. Chose JSON normalisation over reflection: stripKey now normalises any composite value (struct, pointer, map/slice/array of structs) through a json.Marshal + UseNumber decode and then strips it with the existing map/slice recursion, so there is one definition of the delta vocabulary (json tags, omitempty, embedded fields, self-marshalling types such as time.Time) instead of a reflective second implementation that could drift. UseNumber keeps number literals exact, so only the Go type changes, never the wire bytes. stripKey now returns an error and SanitizeDelta fails closed - an unmarshalable value is dropped and logged rather than broadcast unexamined. Nothing about what is stored changed: core.UpdateField still calls setField with the caller's value before publishing, and stripKey copies rather than mutates. RED proven twice against the pre-fix delta.go: the new events tests showed the whole models.Stage (stage + scene privateData) surviving SanitizeDelta, and the new repo/game contract test showed the same stage broadcast verbatim on both the memory and MongoDB backends. Confirmed the sibling has no equivalent hole: a removed entry is a path and nothing else, so pathHasSegment is the whole of its sanitisation. Verified: go build ./... && go vet ./... && gofmt -l internal/ && go test -race ./... all green, no skips.


### 2026-09-13T23:53:52.978Z - Proof completeness set PROVEN: 5 of 5 criteria; structs, pointers, maps and slices of structs, nested and embedded fields all covered

### 2026-09-13T23:53:53.056Z - Proof feature-availability set PROVEN: The end-to-end test exercises UpdateField on both the memory and mongodb backends, so the real publish path is covered rather than only the helper

### 2026-09-13T23:53:53.128Z - Proof robustness set PROVEN: RED proved by restoring the pre-fix delta.go from HEAD: all ten table cases failed, publishing the stage and scene private stores verbatim, and the store contract test failed on both backends

### 2026-09-13T23:53:53.201Z - Proof resilience set PROVEN: SanitizeDelta now fails closed - a value that cannot be rendered as JSON cannot be shown to be private-free, so that path is dropped and logged rather than broadcast unexamined, and loses nothing because such a value could never have reached a client anyway

### 2026-09-13T23:53:53.280Z - Proof security set PROVEN: Closes a P1 sanitization bypass: UpdateField published its value verbatim and stripKey returned any non-map non-slice untouched, so a struct carrying PrivateData reached every player; two existing stage callers already pass such structs

### 2026-09-13T23:53:53.359Z - Proof defense-in-depth set PROVEN: A test asserts the delta agrees with GameService.Sanitize compared through the JSON view on both sides, which also catches over-stripping, and CLAUDE.md requires those two to agree

### 2026-09-13T23:53:53.438Z - Proof input-validation set PROVEN: Value preservation is pinned including MaxInt64 via UseNumber, byte slices, and embedded, renamed, omitted and hidden fields, so only the Go type and key order change and never the wire bytes

### 2026-09-13T23:53:53.523Z - Proof thread-safety set PROVEN: A test asserts the caller's own value is not mutated; suite green under -race

### 2026-09-13T23:53:53.624Z - Proof configurability set NOT_APPLICABLE: No configuration; the rule is fixed by what the client is allowed to see
