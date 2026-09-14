---
id: 085-4799
title: SSE on native via react-native-sse
status: complete
priority: P2
type: feature
created: "2026-09-14T00:50:10.419Z"
updated: "2026-09-14T00:54:27.639Z"
dependencies: []
started_at: "2026-09-14T00:50:14.005Z"
completed_at: "2026-09-14T00:54:27.639Z"
---

# SSE on native via react-native-sse

## Problem Statement

The SSE+REST channel is web-only. React Native ships no EventSource, so SseRestTransport.connect rejects and the supervisor skips the channel on iOS and Android, leaving native clients with two transports instead of three. react-native-sse is pure JS over XMLHttpRequest with no native module, so it needs no config plugin and no prebuild - but it must not be used on web, because it parses by indexing into xhr.responseText and never truncates it, so a long-lived stream grows unbounded where the browser EventSource does not.

## Acceptance Criteria

- [x] react-native-sse is added as a dependency and the lockfile is updated
- [x] Native uses react-native-sse and web keeps the browser EventSource, so the unbounded responseText buffer is never taken on web
- [x] On native the session token travels in an Authorization header rather than the URL query string, which the server already prefers
- [x] The library pollingInterval auto-reconnect is disabled, so the failover supervisor stays the single backoff owner
- [x] A deliberate close does not surface as a failure, matching every other transport
- [x] The stream implementation is injectable, so connect, open, message and error are covered by tests under the node runner for the first time
- [x] SSE is an eligible failover candidate on native once a session token exists
- [REJECTED] [MANUAL] The channel opens and carries deltas on a real device under Expo SDK 53 New Architecture ([MANUAL] Needs a real iOS/Android device build; cannot be run in this environment)
- [x] VERIFY: cd client && rm -rf node_modules && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test && pnpm run lint

## Proof

- [x] [completeness] Completeness (8 of 9 criteria verified; criterion 8 is [MANUAL] and listed in QA. SSE tests 10 to 16; clean-install run 402/402)
- [x] [feature-availability] Feature availability (tsc --listFiles confirms sse-stream.native.ts compiles against react-native-sse/index.d.ts; SocketProvider needed no change because Metro swaps the module, not the call site)
- [x] [robustness] Robustness (Tests cover open, inbound frames, failure before open, failure after open, owner close, and close while connecting. A null-data keep-alive frame is not forwarded)
- [x] [resilience] Resilience (pollingInterval 0 verified against the library source （_pollAgain no-ops for a non-positive interval）, so there is exactly one backoff owner. If Metro's platform resolution ever fails, native falls back to the web file, EventSource is undefined, connect rejects, and the channel simply leaves the rotation)
- [x] [security] Security (Native puts the bearer token in an Authorization header, off the URL entirely. The error reason is a fixed string rather than the library payload, which carries responseText from an authenticated URL. Test asserts no rejection contains the token)
- [x] [defense-in-depth] Defense in depth (Both implementations detach their listeners before close, so an aborted XHR surfacing a late error cannot be read as a channel failure. A factory that throws is caught and turned into a rejection)
- [x] [input-validation] Input validation (Token is encodeURIComponent'd on the web path; the outbound REST leg keeps the existing route-name gate)
- [~] [thread-safety] Thread safety (Single-threaded event loop; one stream is owned by one transport and recreated per connect)
- [x] [configurability] Configurability (StreamFactory is a constructor parameter defaulting to the platform implementation, so a caller or test can supply its own)

## QA

- [ ] Build on a real iOS/Android device and confirm the SSE channel opens and carries deltas under Expo SDK 53 New Architecture

## Work Log

### 2026-09-14T00:54:12.121Z - Added react-native-sse@1.2.1 (MIT, zero deps, pure JS over XHR - no native module, no config plugin, no prebuild). Introduced an EventStream/StreamFactory seam in sse-stream.ts, with sse-stream.native.ts as the Metro platform-extension override: web keeps the browser EventSource, native gets react-native-sse. That split is deliberate - the library indexes into xhr.responseText and never truncates it, so a long-lived stream grows unbounded, a cost only acceptable on the last-resort channel. Native sends the token as an Authorization header, so it leaves the URL entirely; no server change was needed because sse.go tokenFrom already prefers the header. pollingInterval 0 disables the library's own 5s reconnect, keeping the supervisor the single backoff owner. The seam is injectable, so connect/open/message/error/close are now covered: SSE tests went 10 to 16. tsc --listFiles confirms sse-stream.native.ts is typechecked against react-native-sse's own index.d.ts. Also fixed a red client suite: commit c3efbe0 updated the example config's Lua comment but left root config.json claiming a Go move handler that the same commit deleted, breaking 'both shipped configs carry the same layout'. Clean install: 402/402 pass, typecheck clean, lint 0 errors.


### 2026-09-14T00:54:26.432Z - Proof completeness set PROVEN: 8 of 9 criteria verified; criterion 8 is [MANUAL] and listed in QA. SSE tests 10 to 16; clean-install run 402/402

### 2026-09-14T00:54:26.587Z - Proof feature-availability set PROVEN: tsc --listFiles confirms sse-stream.native.ts compiles against react-native-sse/index.d.ts; SocketProvider needed no change because Metro swaps the module, not the call site

### 2026-09-14T00:54:26.733Z - Proof robustness set PROVEN: Tests cover open, inbound frames, failure before open, failure after open, owner close, and close while connecting. A null-data keep-alive frame is not forwarded

### 2026-09-14T00:54:26.835Z - Proof resilience set PROVEN: pollingInterval 0 verified against the library source (_pollAgain no-ops for a non-positive interval), so there is exactly one backoff owner. If Metro's platform resolution ever fails, native falls back to the web file, EventSource is undefined, connect rejects, and the channel simply leaves the rotation

### 2026-09-14T00:54:26.948Z - Proof security set PROVEN: Native puts the bearer token in an Authorization header, off the URL entirely. The error reason is a fixed string rather than the library payload, which carries responseText from an authenticated URL. Test asserts no rejection contains the token

### 2026-09-14T00:54:27.047Z - Proof defense-in-depth set PROVEN: Both implementations detach their listeners before close, so an aborted XHR surfacing a late error cannot be read as a channel failure. A factory that throws is caught and turned into a rejection

### 2026-09-14T00:54:27.159Z - Proof input-validation set PROVEN: Token is encodeURIComponent'd on the web path; the outbound REST leg keeps the existing route-name gate

### 2026-09-14T00:54:27.291Z - Proof thread-safety set NOT_APPLICABLE: Single-threaded event loop; one stream is owned by one transport and recreated per connect

### 2026-09-14T00:54:27.389Z - Proof configurability set PROVEN: StreamFactory is a constructor parameter defaulting to the platform implementation, so a caller or test can supply its own
