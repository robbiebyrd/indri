---
id: 032-0be8
title: Field pickers for every descriptor kind
status: complete
priority: P2
type: feature
created: "2026-09-10T00:58:00.969Z"
updated: "2026-09-12T02:53:32.009Z"
dependencies: ["031"]
plan: plans/layout-authoring-editor.md
plan_step: Step 8
depends_on: ["stories/031-a610-pending-P2-config-panel-drawn-from-field-descriptors.md"]
started_at: "2026-09-12T02:35:49.726Z"
completed_at: "2026-09-12T02:53:32.009Z"
---

# Field pickers for every descriptor kind

## Problem Statement

Eight descriptor kinds need cross-platform controls. There is no DOM, and the repo already has a dependency-free Select built for exactly this reason, so no picker library should be added.

## Acceptance Criteria

- [x] VERIFY: cd client && pnpm test
- [x] Pickers exist for text, number, color, boolean, date, select, multiselect and uri
- [x] components/display/select.tsx is reused for select and extended for multiselect rather than adding a picker library
- [x] Parse and format logic lives in pure helpers beside the components so it is testable without a renderer, matching the core/renderer split used throughout Plan A
- [x] Tested pure helpers cover: colour parse and normalise (#rgb to #rrggbb, invalid rejected), number clamp to min/max, date ISO round-trip, multiselect dedupe and order stability
- [x] No DOM inputs anywhere
- [REJECTED] [VISUAL] Every picker verified by a human on web, iOS and Android, especially colour and date which have no shared cross-platform primitive ([VISUAL] Every picker needs a human on web, iOS and Android. Colour and date most of all - both are deliberately limited controls (hex field with swatch; segmented Y/M/D) with no shared cross-platform primitive behind them.)

## Files

- client/components/board/editor/pickers/
- client/components/display/select.tsx

## Proof

- [x] [completeness] Completeness (5 of 6 criteria checked; the [VISUAL] one is rejected with a reason. 300/300 tests, typecheck clean, lint still 5 pre-existing warnings.)
- [x] [feature-availability] Feature availability (All eight pickers confirmed present in the emitted web bundle by name, not merely written. The PICKERS mapped type is intact, so a missing picker remains a compile error.)
- [x] [robustness] Robustness (joinIsoDate rejects non-existent dates instead of rolling them forward, and builds no Date object, avoiding both the rollover and the two-digit-year trap. Unparseable colours never reach backgroundColor, where they are a hard error on native.)
- [x] [resilience] Resilience (color, date and uri hold a local draft and commit on blur rather than per keystroke, so a seven-character hex value is one op rather than seven full state replays on every connected device.)
- [~] [security] Security (Client-side controls; every value is re-validated by the schema and then by the Go handler.)
- [x] [defense-in-depth] Defense in depth (No picker library and no DOM inputs: the existing dependency-free Select is reused and extended, which is also what keeps this working on native.)
- [x] [input-validation] Input validation (Pickers never validate - they hand raw values to applyFieldEdit, which checks against the widget's own schema. A second validator in the picker would be a divergent source of truth. Typed values are NOT clamped so an over-limit entry is reported rather than silently altered; only steppers clamp, because a tap is not an assertion about a value.)
- [~] [thread-safety] Thread safety (React component state and one debounce timer.)
- [x] [configurability] Configurability (Descriptor options （min, max, step, maxLength, multiline, accept） drive each control; a widget gets working controls with no edit here.)

## QA

- [ ] 300/300 tests, typecheck clean, lint unchanged at 5 warnings
- [ ] All eight pickers confirmed in the emitted web bundle; grep shows zero DOM elements in pickers/
- [ ] [VISUAL] every picker still needs a human, colour and date most of all

## Work Log

### 2026-09-12T02:53:07.804Z - Eight real pickers replacing PlaceholderPicker, plus parse.ts holding every parse/format rule renderer-free (30 tests). PickerProps moved to props.ts so the table and the pickers do not import each other. MultiSelect added to components/display/select.tsx, sharing an internal OptionList with Select so the two row styles cannot drift; rows got minHeight 44. COLOUR is a hex field with a live swatch: #ABC normalises to #aabbcc on commit so two spellings of one colour do not read as two colours in a delta. It is NOT a wheel or eyedropper, and crucially values the platform accepts but hex cannot express (red, rgba(), hsl()) pass through UNCHANGED with a dashed no-preview swatch - the text widget's schema is deliberately z.string(), so hex-only emission would have made color:'red' uneditable. DATE is segmented Y/M/D in ISO order committing YYYY-MM-DD; joinIsoDate refuses dates that do not exist (30 Feb, 29 Feb 2026) instead of rolling forward the way new Date does, and constructs no Date at all to dodge both the rollover and the Date.UTC(26,..)===1926 trap. It is NOT a calendar. Debounce discipline is the subtle part: text and number emit per keystroke because the coalescer debounces them, but color/date/uri are text entry that is NOT a debounced kind, so they hold a local draft and commit on blur - otherwise typing #aabbcc would be seven layout ops and seven full state replays on every connected device. Two non-obvious behaviours the agent flagged for review and I accept: DraftInput adopts a changed value prop only while unfocused (otherwise the server echo fights the caret every keystroke, at the cost of dropping another host's edit to a field you are typing in), and NumberPicker holds the only non-authoritative state in any picker because five quick + taps inside one debounce window would otherwise each compute value+1 and produce a stepper that counts to one. All eight confirmed present in the emitted web bundle.


### 2026-09-12T02:53:08.344Z - Proof completeness set PROVEN: 5 of 6 criteria checked; the [VISUAL] one is rejected with a reason. 300/300 tests, typecheck clean, lint still 5 pre-existing warnings.

### 2026-09-12T02:53:08.427Z - Proof feature-availability set PROVEN: All eight pickers confirmed present in the emitted web bundle by name, not merely written. The PICKERS mapped type is intact, so a missing picker remains a compile error.

### 2026-09-12T02:53:08.508Z - Proof input-validation set PROVEN: Pickers never validate - they hand raw values to applyFieldEdit, which checks against the widget's own schema. A second validator in the picker would be a divergent source of truth. Typed values are NOT clamped so an over-limit entry is reported rather than silently altered; only steppers clamp, because a tap is not an assertion about a value.

### 2026-09-12T02:53:08.584Z - Proof robustness set PROVEN: joinIsoDate rejects non-existent dates instead of rolling them forward, and builds no Date object, avoiding both the rollover and the two-digit-year trap. Unparseable colours never reach backgroundColor, where they are a hard error on native.

### 2026-09-12T02:53:08.661Z - Proof resilience set PROVEN: color, date and uri hold a local draft and commit on blur rather than per keystroke, so a seven-character hex value is one op rather than seven full state replays on every connected device.

### 2026-09-12T02:53:08.735Z - Proof security set NOT_APPLICABLE: Client-side controls; every value is re-validated by the schema and then by the Go handler.

### 2026-09-12T02:53:08.811Z - Proof defense-in-depth set PROVEN: No picker library and no DOM inputs: the existing dependency-free Select is reused and extended, which is also what keeps this working on native.

### 2026-09-12T02:53:08.883Z - Proof thread-safety set NOT_APPLICABLE: React component state and one debounce timer.

### 2026-09-12T02:53:08.957Z - Proof configurability set PROVEN: Descriptor options (min, max, step, maxLength, multiline, accept) drive each control; a widget gets working controls with no edit here.
