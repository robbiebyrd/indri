# Compact delta protocol — declined (story 083)

Status: **declined** by Boss (083 worklog). Filed as a plan so the option is not lost, not scheduled.

## What it is (origin/main)

`internal/services/events/{positional,clientview,wire}.go` replace the dotted-path JSON delta with
positional integer-array paths over MessagePack, plus layout frames sent separately and a keyframe
re-sent when the game's shape changes. `?debug=1` on a connection switches it back to JSON.

## Why it was declined

Measured on the tictactoe config, nine moves (`.integration/scratch/measure.py`, `stream.py`):

| delta encoding | raw bytes | with permessage-deflate |
|---|---|---|
| today: JSON, dotted paths | 2007 | 292 |
| origin's compact format | 873 | 205 |

Deflate alone takes 85% off. The whole migration then buys a further 87 bytes per game (~30% of the
deflated figure), because positional paths and deflate remove the same redundancy — repeated path
strings — and do not stack.

## What adopting it would cost

- The client parser (`client/services/game-state-parser.ts`) and every `scripttest` fixture.
- Escaped-key path support: origin splits paths on a plain dot.
- `events.SanitizeDelta` would need a pair-array form proven to strip exactly what the map form strips,
  or keyframes and deltas disagree on what is private.

## Revisit if

A client platform turns out not to negotiate permessage-deflate *and* application-layer deflate
(story 093's fallback) is rejected — then the raw column is what players pay, and 2007 vs 873 matters.
