import test from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {join} from "node:path";

import {z} from "zod";

import {
    applyFieldEdit,
    buildConfigEdit,
    coerceFieldValue,
    ConfigEditCoalescer,
    isDebouncedKind,
    isDrawableField,
} from "./panel.ts";
import {setWidgetConfig} from "./ops.ts";
import {
    IMAGE_DEFAULTS,
    ImageConfigSchema,
    imageDefinition,
} from "../../components/board/widgets/image/definition.ts";
import {
    SUBGRID_DEFAULTS,
    subGridDefinition,
} from "../../components/board/widgets/subgrid/definition.ts";
import {
    TEXT_DEFAULTS,
    TextConfigSchema,
    textDefinition,
} from "../../components/board/widgets/text/definition.ts";

import type {Timers} from "./panel.ts";
import type {EditContext, LayoutSocket} from "./ops.ts";
import type {FieldDescriptor, FieldKind} from "../registry/fields.ts";

// --- harness ----------------------------------------------------------------

/** Captures what would go on the wire, exactly as ops.node-test.ts does. */
function fakeSocket(): LayoutSocket & {sent: Record<string, unknown>[]} {
    const sent: Record<string, unknown>[] = [];
    return {sent, send: (m) => void sent.push(m as Record<string, unknown>)};
}

const CTX: EditContext = {
    gameCode: "ABCD",
    sceneId: "board",
    grid: {cols: 12, rows: 12},
    siblings: [],
};

/**
 * Deterministic timers. The debounce is about ordering and coalescing, not
 * about a wall clock, so the tests drive the clock rather than sleeping.
 */
function fakeTimers() {
    const queued: {id: number; run: () => void}[] = [];
    let nextId = 1;

    const timers: Timers = {
        set(run) {
            const id = nextId++;
            queued.push({id, run});
            return id;
        },
        clear(handle) {
            const i = queued.findIndex((q) => q.id === handle);
            if (i >= 0) queued.splice(i, 1);
        },
    };

    return {
        timers,
        /** Fire everything currently due. */
        tick() {
            for (const q of queued.splice(0)) q.run();
        },
        armed: () => queued.length,
    };
}

/** Wire the coalescer to a socket the way `ConfigPanel` does. */
function editor(ws: LayoutSocket, widgetId = "w1", delayMs = 250) {
    const clock = fakeTimers();
    const coalescer = new ConfigEditCoalescer(
        (patch) => setWidgetConfig(ws, CTX, widgetId, patch),
        delayMs,
        clock.timers,
    );

    return {coalescer, clock};
}

function fieldOf(fields: FieldDescriptor[], key: string): FieldDescriptor {
    const found = fields.find((f) => f.key === key);
    if (found === undefined) assert.fail(`no field descriptor for "${key}"`);

    return found;
}

function sentConfig(message: Record<string, unknown> | undefined): Record<string, unknown> {
    assert.ok(message, "expected a layout op on the socket");

    return (message as {config: Record<string, unknown>}).config;
}

// --- 1. every kind maps to a picker -----------------------------------------

// A mapped type over the union, mirroring the one `config-panel.tsx` uses for
// PICKERS. Adding a kind to FieldDescriptor without adding it here does not
// compile, which is the first half of the guarantee.
const ALL_KINDS: {[K in FieldKind]: true} = {
    text: true,
    number: true,
    color: true,
    boolean: true,
    date: true,
    select: true,
    multiselect: true,
    uri: true,
};

/**
 * The second half of the guarantee, at runtime, against the real file.
 *
 * The PICKERS table cannot be imported here: it lives in a `.tsx` that pulls in
 * react-native, and the bare-Node runner can load neither JSX nor react-native.
 * Reading the declaration out of the source is therefore the only way to assert
 * the actual table — asserting against a copy of it in this file would prove
 * nothing about the panel. The literal is written one key per line precisely so
 * this stays a stable parse rather than a guess.
 */
function declaredPickerKinds(): string[] {
    const source = readFileSync(
        // import.meta.dirname rather than a URL: the DOM `URL` type that
        // react-native's lib pulls in does not unify with node:url's, so
        // fileURLToPath will not typecheck here.
        join(import.meta.dirname, "../../components/board/editor/config-panel.tsx"),
        "utf8",
    );

    // From `const PICKERS...= {` to the first line that is just `}`. The type
    // annotation contains braces but never one at the start of a line.
    const body = source.match(/const PICKERS[\s\S]*?=\s*\{([\s\S]*?)\n\}/);
    if (body === null) assert.fail("config-panel.tsx no longer declares a PICKERS table");

    return [...body[1].matchAll(/^\s+([A-Za-z_$][\w$]*)\s*:/gm)].map((m) => m[1]);
}

test("every FieldDescriptor kind has a picker in the panel's dispatch table", () => {
    const declared = declaredPickerKinds();

    assert.deepEqual(
        declared.slice().sort(),
        Object.keys(ALL_KINDS).sort(),
        "a descriptor kind with no picker draws nothing at all for that config key",
    );
    assert.equal(new Set(declared).size, declared.length, "a kind is listed twice");
});

// --- 2. an edit becomes a payload the widget's own schema accepts ------------

test("a text widget edit produces a setWidgetConfig payload its schema accepts", () => {
    const ws = fakeSocket();
    const {coalescer, clock} = editor(ws, "title");

    const edit = applyFieldEdit(
        textDefinition,
        TEXT_DEFAULTS,
        fieldOf(textDefinition.fields, "fontSize"),
        // A picker over a numeric keyboard reports a STRING; coercion is what
        // turns it into the number the schema demands.
        "24",
        coalescer,
    );

    if (!edit.ok) assert.fail(edit.error);
    assert.deepStrictEqual(edit.patch, {fontSize: 24});

    clock.tick();
    assert.deepStrictEqual(ws.sent[0], {
        action: "layout", op: "setWidgetConfig", code: "ABCD",
        sceneId: "board", widgetId: "title", config: {fontSize: 24},
    });

    // The config the server will hold after its merge must still parse.
    TextConfigSchema.parse({...TEXT_DEFAULTS, ...sentConfig(ws.sent[0])});
});

test("an image widget select edit produces a payload its schema accepts", () => {
    const ws = fakeSocket();
    const {coalescer, clock} = editor(ws, "hero");

    const edit = applyFieldEdit(
        imageDefinition,
        IMAGE_DEFAULTS,
        fieldOf(imageDefinition.fields, "resizeMode"),
        "cover",
        coalescer,
    );

    if (!edit.ok) assert.fail(edit.error);

    // A select is not free text, so it is not debounced: the op is already out
    // before the clock is touched.
    assert.equal(clock.armed(), 0, "a discrete choice must not wait on a debounce");
    assert.deepStrictEqual(sentConfig(ws.sent[0]), {resizeMode: "cover"});

    ImageConfigSchema.parse({...IMAGE_DEFAULTS, ...sentConfig(ws.sent[0])});
});

// --- 3. an invalid value is reported and not sent ----------------------------

test("a value the descriptor rejects is reported and never reaches the socket", () => {
    const ws = fakeSocket();
    const {coalescer, clock} = editor(ws);

    const notANumber = applyFieldEdit(
        textDefinition, TEXT_DEFAULTS, fieldOf(textDefinition.fields, "fontSize"),
        "huge", coalescer,
    );
    const outOfRange = applyFieldEdit(
        textDefinition, TEXT_DEFAULTS, fieldOf(textDefinition.fields, "fontSize"),
        "9000", coalescer,
    );
    const notAnOption = applyFieldEdit(
        textDefinition, TEXT_DEFAULTS, fieldOf(textDefinition.fields, "align"),
        "diagonal", coalescer,
    );

    assert.equal(notANumber.ok, false);
    assert.equal(outOfRange.ok, false);
    assert.equal(notAnOption.ok, false);

    clock.tick();
    assert.equal(ws.sent.length, 0, "an invalid value must not be on the wire, even briefly");
    assert.equal(coalescer.hasPending(), false, "an invalid value must not even be queued");
});

test("a rejection names the field, so the panel has something to display", () => {
    const edit = buildConfigEdit(
        textDefinition, TEXT_DEFAULTS, fieldOf(textDefinition.fields, "fontSize"), "9000",
    );

    if (edit.ok) assert.fail("9000 is above the descriptor's max of 512");
    assert.match(edit.error, /Size/);
    assert.match(edit.error, /512/);
});

// The descriptor and the schema are two different authorities, and the schema
// is the one that also runs on the server. A value the descriptor waves through
// must still be refused if the schema would reject it.
test("a value only the schema rejects is still reported and not sent", () => {
    const ws = fakeSocket();
    const {coalescer, clock} = editor(ws);

    // Deliberately looser descriptor than schema: `kind: "text"` accepts any
    // string, the schema accepts lowercase letters only.
    const def = {schema: z.object({slug: z.string().regex(/^[a-z]+$/)}).strict()};
    const slug: FieldDescriptor = {key: "slug", label: "Slug", kind: "text"};

    const bad = applyFieldEdit(def, {slug: "ok"}, slug, "Not A Slug", coalescer);
    assert.equal(bad.ok, false);

    clock.tick();
    assert.equal(ws.sent.length, 0);

    const good = applyFieldEdit(def, {slug: "ok"}, slug, "better", coalescer);
    assert.equal(good.ok, true);
    clock.tick();
    assert.equal(ws.sent.length, 1);
});

// `current` comes from the server and may already be failing on a key this
// panel is not editing. Refusing the edit for that reason would wedge the panel
// with no input capable of fixing it.
test("an unrelated pre-existing problem does not block the key being edited", () => {
    const def = {schema: z.object({a: z.string(), b: z.number()}).strict()};
    const a: FieldDescriptor = {key: "a", label: "A", kind: "text"};

    // `b` is already the wrong type in the config the server sent.
    const edit = buildConfigEdit(def, {a: "x", b: "not a number"}, a, "y");

    if (!edit.ok) assert.fail(edit.error);
    assert.deepStrictEqual(edit.patch, {a: "y"});
});

// --- 4. debounced edits coalesce --------------------------------------------

test("a burst of keystrokes becomes one op carrying the last value", () => {
    const ws = fakeSocket();
    const {coalescer, clock} = editor(ws, "title");
    const text = fieldOf(textDefinition.fields, "text");

    for (const typed of ["h", "he", "hel", "hell", "hello"]) {
        applyFieldEdit(textDefinition, TEXT_DEFAULTS, text, typed, coalescer);
        assert.equal(ws.sent.length, 0, "nothing may go out while the burst is still arriving");
    }

    assert.equal(clock.armed(), 1, "each keystroke must reset the timer, not add one");

    clock.tick();
    assert.equal(ws.sent.length, 1, "five keystrokes are one op, not five");
    assert.deepStrictEqual(sentConfig(ws.sent[0]), {text: "hello"});
});

test("an immediate edit carries the pending one with it, preserving order", () => {
    const ws = fakeSocket();
    const {coalescer, clock} = editor(ws, "title");

    applyFieldEdit(
        textDefinition, TEXT_DEFAULTS, fieldOf(textDefinition.fields, "text"), "hi", coalescer,
    );
    // A dropdown fires while the text is still pending. Sending it on its own
    // would let it overtake the text, and the server applies ops in order.
    applyFieldEdit(
        textDefinition, TEXT_DEFAULTS, fieldOf(textDefinition.fields, "align"), "center", coalescer,
    );

    assert.equal(ws.sent.length, 1);
    assert.deepStrictEqual(sentConfig(ws.sent[0]), {text: "hi", align: "center"});

    clock.tick();
    assert.equal(ws.sent.length, 1, "the pending text was already sent; it must not be sent twice");
});

test("flush sends what is pending, and cancel throws it away", () => {
    const ws = fakeSocket();
    const {coalescer} = editor(ws, "title");
    const text = fieldOf(textDefinition.fields, "text");

    applyFieldEdit(textDefinition, TEXT_DEFAULTS, text, "kept", coalescer);
    coalescer.flush();
    assert.equal(ws.sent.length, 1);

    // Flushing an empty coalescer must not emit an empty config op.
    coalescer.flush();
    assert.equal(ws.sent.length, 1);

    applyFieldEdit(textDefinition, TEXT_DEFAULTS, text, "dropped", coalescer);
    coalescer.cancel();
    coalescer.flush();
    assert.equal(ws.sent.length, 1);
});

test("only text and number edits are debounced", () => {
    assert.equal(isDebouncedKind("text"), true);
    assert.equal(isDebouncedKind("number"), true);
    for (const kind of ["color", "boolean", "date", "select", "multiselect", "uri"] as const) {
        assert.equal(isDebouncedKind(kind), false, `${kind} is a whole value per gesture`);
    }
});

// --- 5. only the changed key is sent -----------------------------------------

test("the op carries only the key that changed, not the whole config", () => {
    const ws = fakeSocket();
    const {coalescer, clock} = editor(ws, "title");

    assert.ok(
        Object.keys(TEXT_DEFAULTS).length > 1,
        "this test is only meaningful while the text config has several keys",
    );

    applyFieldEdit(
        textDefinition, TEXT_DEFAULTS, fieldOf(textDefinition.fields, "color"), "#ff0000", coalescer,
    );
    clock.tick();

    assert.deepStrictEqual(sentConfig(ws.sent[0]), {color: "#ff0000"});
});

// --- coercion details --------------------------------------------------------

test("a number field accepts a numeric string and refuses an empty one", () => {
    const size: FieldDescriptor = {key: "size", label: "Size", kind: "number", min: 1, max: 10};

    assert.deepStrictEqual(coerceFieldValue(size, " 4 "), {ok: true, value: 4});
    assert.deepStrictEqual(coerceFieldValue(size, 4), {ok: true, value: 4});
    // An empty box is not zero: coercing it would write a value nobody typed.
    assert.equal(coerceFieldValue(size, "").ok, false);
    assert.equal(coerceFieldValue(size, "0").ok, false);
    assert.equal(coerceFieldValue(size, "11").ok, false);
    assert.equal(coerceFieldValue(size, true).ok, false);
});

test("a text field honours its own maxLength", () => {
    const body: FieldDescriptor = {key: "body", label: "Body", kind: "text", maxLength: 3};

    assert.deepStrictEqual(coerceFieldValue(body, "abc"), {ok: true, value: "abc"});
    assert.equal(coerceFieldValue(body, "abcd").ok, false);
    assert.equal(coerceFieldValue(body, 12).ok, false);
});

test("a multiselect keeps order, drops duplicates and refuses unknown choices", () => {
    const tags: FieldDescriptor = {
        key: "tags",
        label: "Tags",
        kind: "multiselect",
        options: [{label: "A", value: "a"}, {label: "B", value: "b"}],
    };

    assert.deepStrictEqual(coerceFieldValue(tags, ["b", "a", "b"]), {ok: true, value: ["b", "a"]});
    assert.equal(coerceFieldValue(tags, ["c"]).ok, false);
    assert.equal(coerceFieldValue(tags, "a").ok, false);
});

test("a boolean field refuses anything that is not a boolean", () => {
    const on: FieldDescriptor = {key: "on", label: "On", kind: "boolean"};

    assert.deepStrictEqual(coerceFieldValue(on, false), {ok: true, value: false});
    assert.equal(coerceFieldValue(on, "true").ok, false);
});

// --- the sub-grid's undrawable keys ------------------------------------------

// `subgrid` declares `grid` and `widgets` as `kind: "text"` because the
// descriptor vocabulary has no honest kind for either. The panel must not draw
// a text box over them; see `isDrawableField` for why this is a structural test
// rather than a new FieldDescriptor kind.
test("the sub-grid's composite config keys are not drawable as text boxes", () => {
    const grid = fieldOf(subGridDefinition.fields, "grid");
    const widgets = fieldOf(subGridDefinition.fields, "widgets");

    assert.equal(isDrawableField(grid, SUBGRID_DEFAULTS.grid), false);
    assert.equal(isDrawableField(widgets, SUBGRID_DEFAULTS.widgets), false);
});

test("every real primitive field stays drawable", () => {
    const values: Record<string, unknown> = {...TEXT_DEFAULTS, ...IMAGE_DEFAULTS};

    for (const field of [...textDefinition.fields, ...imageDefinition.fields]) {
        assert.equal(
            isDrawableField(field, values[field.key]),
            true,
            `${field.key} should be drawable`,
        );
    }
});

test("a missing value is drawable, and a multiselect's array is too", () => {
    const name: FieldDescriptor = {key: "name", label: "Name", kind: "text"};
    const tags: FieldDescriptor = {
        key: "tags", label: "Tags", kind: "multiselect", options: [{label: "A", value: "a"}],
    };

    assert.equal(isDrawableField(name, undefined), true);
    assert.equal(isDrawableField(tags, ["a"]), true);
    // A descriptor that is simply wrong about its own key is caught by the same
    // rule, which is the point of testing the shape rather than the widget type.
    assert.equal(isDrawableField(name, {oops: 1}), false);
});
