---
id: 096-62ef
title: Show the player why their session moved to another connection
status: pending
priority: P2
type: feature
created: "2026-10-10T01:14:21.000Z"
updated: "2026-10-10T01:14:21.000Z"
dependencies: ["093-2c8a"]
---

# Show the player why their session moved to another connection

## Problem Statement

Story 093 enforces one WebSocket per user: binding a second socket closes the older one with close code 4001 (`StatusSuperseded`). The client's `FailoverTransport.lost` (`client/services/failover-transport.ts`) stops for good on 4001, logs `console.warn`, and emits the reason through `onClose`. But `MessageHandler` (`client/services/message-handler.ts`) only clears its `opened` flag on close, and no screen shows anything. The replaced tab looks frozen: no updates arrive, sends fail, and the player is not told why.

## Questions for Boss

1. What should the replaced client show: a full-screen blocking notice, a banner over the game, or a toast?
2. Wording? For example: "You opened this game somewhere else. This window has stopped."
3. Should the notice offer a "Use this window instead" button? Pressing it would redial and `reconnect`, which takes the session back and closes the *other* window with 4001. That is a deliberate user choice, not a loop, but it does make the other window show the same notice.
4. Should anything be shown on the window that took over (e.g. "Your other window was disconnected"), or nothing?
5. Should the 4002 login-deadline close show anything, or stay silent since the client redials on its next send?
6. Where should this live: in `MessageHandler` as a new state the screens read, or in a React context or hook at the app root?

## Acceptance Criteria

- [ ] Boss's answers recorded in the work log
- [ ] A 4001 close reaches a piece of client state the UI renders, with a node test that a superseded close sets it and a plain close does not
- [ ] The chosen UI renders it (checked on web)
- [ ] If chosen: the "use this window" action redials and reconnects, with a test
- [ ] VERIFY: (cd client && pnpm run typecheck && pnpm test && pnpm run lint)

## Work Log
