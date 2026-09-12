/**
 * The pickers' parse/format contract, tested without a renderer.
 *
 * These are the four behaviours the pickers cannot get wrong without corrupting
 * a layout that then fans out to every connected player: a colour that
 * normalises to the wrong colour, a number that escapes its declared bounds, a
 * date that cannot survive a round trip, and a multiselect that reshuffles
 * itself. Everything else about a picker is pixels, and pixels are verified by
 * a human on three platforms.
 */

import test from "node:test";
import assert from "node:assert/strict";

import {
    BLANK_DATE,
    clampNumber,
    fieldOptions,
    isBlankDate,
    joinIsoDate,
    normalizeHexColor,
    numberRange,
    parseNumberText,
    selectedValues,
    splitIsoDate,
    stepNumber,
    textLimits,
    toggleValue,
    toInputText,
    uriAccept,
} from "./parse.ts";

import type {DateParts} from "./parse.ts";
import type {FieldDescriptor, FieldOption} from "../../../../layout/registry/fields.ts";

// --- 1. colour: parse and normalise -----------------------------------------

test("#rgb expands to #rrggbb by doubling each digit, not by padding it", () => {
    assert.equal(normalizeHexColor("#abc"), "#aabbcc");
    assert.equal(normalizeHexColor("#f00"), "#ff0000");
    // The distinction that makes this worth a test: padding would give #0a0b0c.
    assert.equal(normalizeHexColor("#0ab"), "#00aabb");
});

test("case is normalised so two spellings of one colour compare equal", () => {
    assert.equal(normalizeHexColor("#AABBCC"), "#aabbcc");
    assert.equal(normalizeHexColor("#AaBbCc"), "#aabbcc");
    assert.equal(normalizeHexColor("#ABC"), normalizeHexColor("#aabbcc"));
});

test("a leading hash is optional and surrounding whitespace is ignored", () => {
    assert.equal(normalizeHexColor("aabbcc"), "#aabbcc");
    assert.equal(normalizeHexColor("  #abc  "), "#aabbcc");
});

test("anything that is not hex is rejected rather than guessed at", () => {
    for (const raw of ["", "#", "#ab", "#abcd", "#abcde", "#abcdefa", "#gggggg", "12 34 56"]) {
        assert.equal(normalizeHexColor(raw), undefined, `accepted ${JSON.stringify(raw)}`);
    }
});

test("a colour the platform understands but hex cannot express is reported as not-hex", () => {
    // NOT an error: the schema is a plain string and React Native renders all
    // of these. `undefined` is the picker's cue to pass the text through
    // unchanged and draw no swatch, never its cue to refuse the edit.
    for (const raw of ["red", "rebeccapurple", "rgba(1, 2, 3, 0.5)", "hsl(0 100% 50%)"]) {
        assert.equal(normalizeHexColor(raw), undefined, `claimed ${JSON.stringify(raw)} was hex`);
    }
});

// --- 2. number: clamp to the descriptor's bounds, reject non-numeric text ----

const FONT_SIZE: FieldDescriptor = {
    key: "fontSize",
    label: "Size",
    kind: "number",
    min: 1,
    max: 512,
    step: 1,
};

test("a number descriptor's bounds and step are read off the descriptor", () => {
    assert.deepEqual(numberRange(FONT_SIZE), {min: 1, max: 512, step: 1});
});

test("a descriptor with no step steps by one, and a nonsense step is ignored", () => {
    const bare: FieldDescriptor = {key: "n", label: "N", kind: "number"};
    assert.deepEqual(numberRange(bare), {min: undefined, max: undefined, step: 1});

    const zero: FieldDescriptor = {key: "n", label: "N", kind: "number", step: 0};
    assert.equal(numberRange(zero).step, 1, "a zero step makes the stepper a no-op");

    const backwards: FieldDescriptor = {key: "n", label: "N", kind: "number", step: -2};
    assert.equal(numberRange(backwards).step, 1, "a negative step runs the stepper backwards");
});

test("clamping pulls a value to the nearest bound and leaves an in-range value alone", () => {
    const range = numberRange(FONT_SIZE);

    assert.equal(clampNumber(range, 0), 1);
    assert.equal(clampNumber(range, -9000), 1);
    assert.equal(clampNumber(range, 513), 512);
    assert.equal(clampNumber(range, 16), 16);
    // The bounds themselves are inside the range, not outside it.
    assert.equal(clampNumber(range, 1), 1);
    assert.equal(clampNumber(range, 512), 512);
});

test("a half-open range clamps only on the side it declares", () => {
    assert.equal(clampNumber({min: 0, step: 1}, -5), 0);
    assert.equal(clampNumber({min: 0, step: 1}, 1e9), 1e9);
    assert.equal(clampNumber({max: 10, step: 1}, 1e9), 10);
    assert.equal(clampNumber({max: 10, step: 1}, -1e9), -1e9);
});

test("a stepper tap stops at the bound instead of producing a rejected edit", () => {
    const range = numberRange(FONT_SIZE);

    assert.equal(stepNumber(range, 16, 1), 17);
    assert.equal(stepNumber(range, 16, -1), 15);
    assert.equal(stepNumber(range, 1, -1), 1, "stepping below min must not error, it must stop");
    assert.equal(stepNumber(range, 512, 1), 512);
});

test("a fractional step does not accumulate binary floating point dust", () => {
    // 0.1 + 0.2 is 0.30000000000000004, and that value would be written into
    // the layout and broadcast verbatim.
    assert.equal(stepNumber({step: 0.1}, 0.2, 1), 0.3);
    assert.equal(stepNumber({step: 0.1}, 0.3, -1), 0.2);
});

test("non-numeric text is rejected, and an empty box is not zero", () => {
    for (const raw of ["", "   ", "abc", "12abc", "--3", "NaN", "Infinity", "-Infinity"]) {
        assert.equal(parseNumberText(raw), undefined, `accepted ${JSON.stringify(raw)}`);
    }
});

test("numeric text parses, including negatives, decimals and surrounding space", () => {
    assert.equal(parseNumberText("42"), 42);
    assert.equal(parseNumberText("  16  "), 16);
    assert.equal(parseNumberText("-3.5"), -3.5);
    assert.equal(parseNumberText("0"), 0);
});

// --- 3. date: ISO round trip ------------------------------------------------

test("segments join into a zero-padded ISO date", () => {
    assert.equal(joinIsoDate({year: "2026", month: "2", day: "3"}), "2026-02-03");
    assert.equal(joinIsoDate({year: "26", month: "02", day: "03"}), "0026-02-03");
});

test("an ISO date splits and re-joins to exactly itself", () => {
    for (const iso of ["2026-09-11", "2024-02-29", "1970-01-01", "0001-01-01", "9999-12-31"]) {
        const parts = splitIsoDate(iso);
        assert.notEqual(parts, undefined, `${iso} would not split`);
        assert.equal(joinIsoDate(parts as DateParts), iso);
    }
});

test("segments survive a join and a split unchanged once they are padded", () => {
    const typed: DateParts = {year: "2026", month: "9", day: "1"};
    const iso = joinIsoDate(typed);
    assert.equal(iso, "2026-09-01");

    // The round trip normalises the padding and is then idempotent — which is
    // what stops the panel re-emitting an edit every time a value loads.
    const parts = splitIsoDate(iso as string) as DateParts;
    assert.deepEqual(parts, {year: "2026", month: "09", day: "01"});
    assert.equal(joinIsoDate(parts), iso);
});

test("a date that does not exist is refused, not rolled over into the next month", () => {
    // `new Date("2026-02-30")` answers 2 March. Refusing is the whole point.
    assert.equal(joinIsoDate({year: "2026", month: "2", day: "30"}), undefined);
    assert.equal(joinIsoDate({year: "2026", month: "2", day: "29"}), undefined, "2026 is not a leap year");
    assert.equal(joinIsoDate({year: "2026", month: "4", day: "31"}), undefined, "April has 30 days");
    assert.equal(joinIsoDate({year: "1900", month: "2", day: "29"}), undefined, "1900 is not a leap year");
    assert.equal(joinIsoDate({year: "2000", month: "2", day: "29"}), "2000-02-29", "2000 is a leap year");
});

test("out-of-range and non-numeric segments are refused", () => {
    for (const parts of [
        {year: "2026", month: "0", day: "1"},
        {year: "2026", month: "13", day: "1"},
        {year: "2026", month: "1", day: "0"},
        {year: "0", month: "1", day: "1"},
        {year: "10000", month: "1", day: "1"},
        {year: "", month: "1", day: "1"},
        {year: "20x6", month: "1", day: "1"},
        {year: "2026", month: "-1", day: "1"},
        {year: "2026", month: "1.5", day: "1"},
    ]) {
        assert.equal(joinIsoDate(parts), undefined, `accepted ${JSON.stringify(parts)}`);
    }
});

test("text that is not an ISO date does not split, including a plausible one that is not real", () => {
    for (const iso of ["", "2026-9-1", "2026/09/01", "11-09-2026", "2026-02-30", "2026-13-01"]) {
        assert.equal(splitIsoDate(iso), undefined, `split ${JSON.stringify(iso)}`);
    }
});

test("an untouched date entry is blank, and one typed segment is not", () => {
    assert.equal(isBlankDate(BLANK_DATE), true);
    assert.equal(isBlankDate({year: "  ", month: "", day: " "}), true);
    assert.equal(isBlankDate({year: "2026", month: "", day: ""}), false);
});

// --- 4. multiselect: dedupe and order stability ------------------------------

const OPTIONS: readonly FieldOption[] = [
    {label: "Alpha", value: "a"},
    {label: "Bravo", value: "b"},
    {label: "Charlie", value: "c"},
];

test("a stored selection keeps ITS order, not the option list's", () => {
    // The regression this guards: sorting by option order would turn ["c","a"]
    // into ["a","c"] on every render and publish a move nobody made.
    assert.deepEqual(selectedValues(["c", "a"], OPTIONS), ["c", "a"]);
    assert.deepEqual(selectedValues(["b", "c", "a"], OPTIONS), ["b", "c", "a"]);
});

test("re-reading the same selection is stable, so a re-render emits nothing", () => {
    const stored = ["c", "a", "b"];

    const first = selectedValues(stored, OPTIONS);
    const second = selectedValues(first, OPTIONS);
    const third = selectedValues(second, OPTIONS);

    assert.deepEqual(first, stored);
    assert.deepEqual(second, first);
    assert.deepEqual(third, first);
});

test("duplicates collapse to the FIRST occurrence, keeping the earlier position", () => {
    assert.deepEqual(selectedValues(["c", "a", "c"], OPTIONS), ["c", "a"]);
    assert.deepEqual(selectedValues(["a", "a", "a"], OPTIONS), ["a"]);
});

test("values the option list does not offer are dropped", () => {
    // Otherwise every later toggle would be rejected by coerceFieldValue with
    // an error naming a value the panel never drew.
    assert.deepEqual(selectedValues(["a", "zzz", "b"], OPTIONS), ["a", "b"]);
    assert.deepEqual(selectedValues(["a", 7, null, {}, "b"], OPTIONS), ["a", "b"]);
});

test("a config value that is not an array reads as an empty selection", () => {
    for (const value of [undefined, null, "a", 7, {a: true}]) {
        assert.deepEqual(selectedValues(value, OPTIONS), []);
    }
});

test("toggling on appends and toggling off closes the gap, moving nothing else", () => {
    assert.deepEqual(toggleValue([], "b"), ["b"]);
    assert.deepEqual(toggleValue(["c"], "a"), ["c", "a"], "a new value goes on the end");
    assert.deepEqual(toggleValue(["c", "a", "b"], "a"), ["c", "b"], "removal must not reorder");
});

test("toggling a value on and straight off restores the original selection", () => {
    const before = ["c", "b"];
    const after = toggleValue(toggleValue(before, "a"), "a");

    assert.deepEqual(after, before);
});

test("toggling does not mutate the array it was handed", () => {
    const before = ["c", "b"];
    toggleValue(before, "a");
    toggleValue(before, "c");

    assert.deepEqual(before, ["c", "b"]);
});

// --- 5. descriptor accessors the components lean on -------------------------

test("descriptor-specific properties are read off the right kind and defaulted elsewhere", () => {
    const multiline: FieldDescriptor = {key: "t", label: "T", kind: "text", multiline: true, maxLength: 80};
    assert.deepEqual(textLimits(multiline), {multiline: true, maxLength: 80});
    assert.deepEqual(textLimits(FONT_SIZE), {multiline: false});

    const select: FieldDescriptor = {key: "s", label: "S", kind: "select", options: [...OPTIONS]};
    assert.deepEqual(fieldOptions(select), OPTIONS);

    const multi: FieldDescriptor = {key: "m", label: "M", kind: "multiselect", options: [...OPTIONS]};
    assert.deepEqual(fieldOptions(multi), OPTIONS);
    assert.deepEqual(fieldOptions(FONT_SIZE), []);

    const uri: FieldDescriptor = {key: "u", label: "U", kind: "uri", accept: "video"};
    assert.equal(uriAccept(uri), "video");
    assert.equal(uriAccept(FONT_SIZE), undefined);
});

test("a config value becomes input text only when it is one, never [object Object]", () => {
    assert.equal(toInputText("hello"), "hello");
    assert.equal(toInputText(16), "16");
    assert.equal(toInputText(""), "");
    assert.equal(toInputText(undefined), "");
    assert.equal(toInputText(null), "");
    assert.equal(toInputText({a: 1}), "");
    assert.equal(toInputText(["a"]), "");
    assert.equal(toInputText(Number.NaN), "");
});
