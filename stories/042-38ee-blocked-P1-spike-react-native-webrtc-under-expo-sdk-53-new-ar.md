---
id: 042-38ee
title: "SPIKE: react-native-webrtc under Expo SDK 53 New Architecture"
status: blocked
priority: P1
type: task
created: "2026-09-12T01:27:30.683Z"
updated: "2026-09-13T17:25:53.141Z"
dependencies: ["040-9cec"]
plan: plans/webrtc-transport.md
plan_step: Step 9
depends_on: ["stories/040-9cec-pending-P2-webrtc-wiring-config-and-message-size-guard.md"]
---

# SPIKE: react-native-webrtc under Expo SDK 53 New Architecture

## Problem Statement

Plan blocker P1 Q2. Expo SDK 53 enables New Architecture by default on RN 0.79 with React 19, and react-native-webrtc compatibility with Fabric and bridgeless came back unverified from research. Expo Go can never run this library, so a custom dev client is mandatory. If this does not work the client half of the plan is blocked and the server half still ships.

## Acceptance Criteria

- [REJECTED] [MANUAL] A custom dev client builds with the react-native-webrtc config plugin after expo prebuild (blocked upstream: RN 0.79.6 vendors fmt 11.0.2 which Xcode 26 clang cannot compile; breaks any RN native build on this machine, not WebRTC-specific)
- [REJECTED] [MANUAL] A DataChannel reaches the open state against the server on a real iOS device (unreachable: the dev client cannot be built on this toolchain, see criterion 1)
- [REJECTED] [MANUAL] A DataChannel reaches the open state on a real Android device (unreachable: no Android SDK on this machine, and the same RN/fmt build failure would apply)
- [x] [MANUAL] A DataChannel reaches the open state on web through the shim
- [REJECTED] [MANUAL] If New Architecture fails, newArchEnabled false is tried and the outcome recorded (not reached: newArchEnabled was never the failure; the build never got past compiling React Native's own fmt pod)
- [x] The yes or no result is written back into the plan under Open Questions
- [x] If it cannot be made to work, the spike stops and reports rather than attempting upstream fixes

## Files

- client/app.json
- client/package.json

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

### 2026-09-13T17:25:53.073Z - Automatable half done: deps installed (react-native-webrtc 124.0.8, config plugin 15.0.2, web-shim 1.0.7), plugin added to app.json, newArchEnabled confirmed true, typecheck and all 305 client tests pass. expo-doctor reports react-native-webrtc as Untested on New Architecture - React Native Directory has no verdict, so the automated signal cannot settle it. BLOCKED on hardware: the remaining criteria need a custom dev client on real iOS and Android devices.

### 2026-09-13T18:37:46.712Z - WEB VERIFIED against a live server. Browser did the full non-trickle flow: gathering completed, POST /rtc/offer returned 200 with a 16-candidate answer, the game DataChannel reached open, connectionState connected. Then register, login and create all dispatched over the DataChannel with no new action code, returning the login scene frame, registered true, authenticated true and a full game keyframe. Every inbound frame arrived as ArrayBuffer because pion sends binary, which confirms normaliseMessage is load-bearing. Browser reported sctp.maxMessageSize 262144, but the 16 KiB fallback stays since that is the cross-browser figure. Caveat: this exercised the browser RTCPeerConnection that the web shim re-exports, not an import of the shim itself - nothing in the app imports webrtc-transport.ts yet, so Metro bundling of the shim is still unexercised.

### 2026-09-13T23:27:36.684Z - iOS simulator BLOCKED by a toolchain incompatibility unrelated to react-native-webrtc. React Native 0.79.6 vendors fmt 11.0.2, whose FMT_STRING consteval path Xcode 26's clang rejects: call to consteval function is not a constant expression, in Pods/fmt/include/fmt/format-inl.h. That breaks any RN 0.79 native build on this machine, with or without WebRTC. Not workaroundable from build settings: FMT_USE_CONSTEVAL is set by an unguarded if/elif chain in base.h with no ifndef, so an external define is overridden by the header. Real fixes are upgrading React Native, bumping fmt to 11.1+, or using an older Xcode - all app-wide changes outside this spike. What IS established: web verified end to end; the config plugin injects the iOS usage strings and Android permissions; pods resolve and link react-native-webrtc 124.0.8 with JitsiWebRTC 124.0.2; newArchEnabled true reaches Podfile.properties.json so the build would genuinely exercise the New Architecture. Also required getting here: an iOS 26.5 simulator runtime (8.5 GB) since only 18.6 was installed, and a NETRC directory override because CocoaPods refused on a 644 ~/.netrc.

