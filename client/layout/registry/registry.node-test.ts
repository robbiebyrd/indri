import assert from "node:assert/strict";
import test, {beforeEach} from "node:test";
import {z} from "zod";

import {parseLayout} from "../schema/layout.ts";
import {
    assertDescriptorsMatchSchema,
    clearRegistry,
    getWidget,
    knownWidgetTypes,
    listWidgets,
    registerWidget,
} from "./registry.ts";

import type {FieldDescriptor, FieldKind} from "./fields.ts";
import type {WidgetDefinition} from "./registry.ts";

// --- fixture ----------------------------------------------------------------

// One key per FieldDescriptor kind, so the fixture is also the coverage check
// for the kind union (see the last test in this file).
const FixtureConfigSchema = z.object({
    title: z.string(),
    body: z.string().optional(),
    size: z.number().optional(),
    color: z.string().optional(),
    visible: z.boolean().optional(),
    startsAt: z.string().optional(),
    align: z.enum(["left", "center", "right"]).optional(),
    tags: z.array(z.string()).optional(),
    imageUri: z.string().optional(),
}).strict();

type FixtureConfig = z.infer<typeof FixtureConfigSchema>;

const FIXTURE_FIELDS: FieldDescriptor[] = [
    {key: "title", label: "Title", kind: "text", required: true},
    {key: "body", label: "Body", kind: "text", multiline: true, maxLength: 500},
    {key: "size", label: "Size", kind: "number", min: 8, max: 512, step: 1},
    {key: "color", label: "Colour", kind: "color"},
    {key: "visible", label: "Visible", kind: "boolean"},
    {key: "startsAt", label: "Starts at", kind: "date"},
    {
        key: "align",
        label: "Align",
        kind: "select",
        options: [
            {label: "Left", value: "left"},
            {label: "Centre", value: "center"},
            {label: "Right", value: "right"},
        ],
    },
    {
        key: "tags",
        label: "Tags",
        kind: "multiselect",
        options: [{label: "Hot", value: "hot"}, {label: "Cold", value: "cold"}],
    },
    {key: "imageUri", label: "Image", kind: "uri", accept: "image"},
];

// Every optional key is listed too: `defaults` is what the registry treats as
// the definition's declaration of its config keys.
const FIXTURE_DEFAULTS: FixtureConfig = {
    title: "",
    body: "",
    size: 16,
    color: "#ffffff",
    visible: true,
    startsAt: "2026-01-01",
    align: "left",
    tags: [],
    imageUri: "",
};

/** A fresh definition each time, so a test can mutate its own copy. */
function fixture(type = "fixture"): WidgetDefinition<FixtureConfig> {
    return {
        type,
        schema: FixtureConfigSchema,
        fields: [...FIXTURE_FIELDS],
        defaults: {...FIXTURE_DEFAULTS},
    };
}

beforeEach(clearRegistry);

// --- bidirectional descriptor/schema agreement ------------------------------

test("a definition whose descriptors and schema agree passes", () => {
    assert.doesNotThrow(() => assertDescriptorsMatchSchema(fixture()));
});

test("a config key with no field descriptor fails: the panel could not edit it", () => {
    const def = fixture();
    def.fields = def.fields.filter((f) => f.key !== "color");

    assert.throws(
        () => assertDescriptorsMatchSchema(def),
        /config key "color" has no field descriptor/,
    );
});

test("a required config key with no field descriptor fails", () => {
    const def = fixture();
    def.fields = def.fields.filter((f) => f.key !== "title");

    assert.throws(
        () => assertDescriptorsMatchSchema(def),
        /config key "title" has no field descriptor/,
    );
});

test("a descriptor for a key the schema rejects fails: its value would be discarded", () => {
    const def = fixture();
    def.fields = [...def.fields, {key: "nope", label: "Nope", kind: "text"}];

    assert.throws(
        () => assertDescriptorsMatchSchema(def),
        /field descriptor "nope" edits a config key the schema does not accept/,
    );
});

test("a descriptor for a real key absent from defaults does not read as fictitious", () => {
    // The probe fills such a key with a string sentinel, which is the wrong
    // type for `size`. Zod reports `unrecognized_keys` alongside the resulting
    // `invalid_type`, so the wrong type must not be mistaken for a bad key —
    // and a genuinely fictitious key alongside it must still be caught.
    const def = fixture();
    const {size: _dropped, ...withoutSize} = def.defaults;
    def.defaults = withoutSize;

    assert.doesNotThrow(() => assertDescriptorsMatchSchema(def));

    def.fields = [...def.fields, {key: "nope", label: "Nope", kind: "number"}];
    assert.throws(
        () => assertDescriptorsMatchSchema(def),
        /field descriptor "nope" edits a config key the schema does not accept/,
    );
});

test("two descriptors for one key fail", () => {
    const def = fixture();
    def.fields = [...def.fields, {key: "title", label: "Title again", kind: "text"}];

    assert.throws(
        () => assertDescriptorsMatchSchema(def),
        /two field descriptors both edit config key "title"/,
    );
});

test("a non-strict schema fails: it would make the descriptor check pass vacuously", () => {
    // Same shape, no `.strict()`. Unknown keys are stripped rather than
    // reported, so `nope` below would otherwise look like a valid key.
    const loose = z.object({title: z.string()});
    const def: WidgetDefinition<z.infer<typeof loose>> = {
        type: "loose",
        schema: loose,
        fields: [
            {key: "title", label: "Title", kind: "text"},
            {key: "nope", label: "Nope", kind: "text"},
        ],
        defaults: {title: ""},
    };

    assert.throws(() => assertDescriptorsMatchSchema(def), /it is not \.strict\(\)/);
});

// --- defaults ---------------------------------------------------------------

test("defaults validate against the definition's own schema", () => {
    const def = fixture();
    const parsed = def.schema.safeParse(def.defaults);

    assert.equal(parsed.success, true);
});

test("defaults that violate the schema fail before any descriptor check runs", () => {
    const def = fixture();
    def.defaults = {...def.defaults, size: "big" as unknown as number};

    assert.throws(
        () => assertDescriptorsMatchSchema(def),
        /defaults do not satisfy the widget's own schema/,
    );
});

// --- registration -----------------------------------------------------------

test("registering a duplicate type throws", () => {
    registerWidget(fixture("dup"));

    assert.throws(() => registerWidget(fixture("dup")), /"dup" is already registered/);
});

test("a failed duplicate registration leaves the first definition in place", () => {
    const first = fixture("dup");
    registerWidget(first);
    try {
        registerWidget(fixture("dup"));
    } catch {
        // expected; the point of the test is the state afterwards
    }

    assert.equal(getWidget("dup"), first);
    assert.equal(listWidgets().length, 1);
});

test("getWidget on an unregistered type returns undefined rather than throwing", () => {
    assert.equal(getWidget("never-registered"), undefined);
});

test("listWidgets returns definitions in registration order", () => {
    registerWidget(fixture("a"));
    registerWidget(fixture("b"));

    assert.deepEqual(listWidgets().map((d) => d.type), ["a", "b"]);
});

// --- knownWidgetTypes / parseLayout ------------------------------------------

test("knownWidgetTypes returns exactly the registered types", () => {
    registerWidget(fixture("text"));
    registerWidget(fixture("image"));

    assert.deepEqual([...knownWidgetTypes()].sort(), ["image", "text"]);
});

test("knownWidgetTypes is a copy: mutating the result cannot change the registry", () => {
    registerWidget(fixture("text"));
    (knownWidgetTypes() as Set<string>).add("smuggled");

    assert.deepEqual([...knownWidgetTypes()], ["text"]);
});

function layoutWithTypes(...types: string[]): unknown {
    const widgets: Record<string, unknown> = {};
    types.forEach((type, i) => {
        widgets[`w${i}`] = {type, placement: {kind: "grid", col: i, row: 0, w: 1, h: 1}};
    });

    return {grid: {cols: 12, rows: 12}, scenes: {board: {widgets}}};
}

test("parseLayout warns about a type the registry does not know", () => {
    registerWidget(fixture("text"));

    const {layout, issues} = parseLayout(layoutWithTypes("nope"), {
        knownWidgetTypes: knownWidgetTypes(),
    });

    // A warning, not an error: the rest of the board still renders.
    assert.ok(layout);
    assert.deepEqual(issues.map((i) => i.severity), ["warning"]);
    assert.match(issues[0].message, /unknown widget type "nope"/);
    assert.equal(issues[0].path, "scenes.board.widgets.w0.type");
});

test("parseLayout accepts a registered type with no issues", () => {
    registerWidget(fixture("text"));

    const {issues} = parseLayout(layoutWithTypes("text"), {
        knownWidgetTypes: knownWidgetTypes(),
    });

    assert.deepEqual(issues, []);
});

// --- field kind coverage ------------------------------------------------------

// A Record over the union, so adding a kind to FieldDescriptor without adding
// it here is a TYPE error, and adding it here without putting it in the
// fixture is a TEST failure. Either way a new kind cannot arrive uncovered.
const ALL_KINDS: Record<FieldKind, true> = {
    text: true,
    number: true,
    color: true,
    boolean: true,
    date: true,
    select: true,
    multiselect: true,
    uri: true,
};

test("every FieldDescriptor kind appears in the fixture definition", () => {
    const covered = new Set(FIXTURE_FIELDS.map((f) => f.kind));

    assert.deepEqual(Object.keys(ALL_KINDS).sort(), [...covered].sort());
});
