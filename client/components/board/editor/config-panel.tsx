/**
 * The auto-drawn config panel: one input per field descriptor.
 *
 * This file is deliberately thin. Everything that can be decided without a
 * renderer — coercion, validation, debouncing — lives in `layout/edit/panel.ts`
 * so the bare-Node test runner can exercise it. What is left here is the
 * dispatch from a descriptor's `kind` to the control that draws it, plus the
 * error and pending-edit state that only a mounted component has.
 *
 * The controls themselves are in `editor/pickers/*`, and their own parse and
 * format logic is in `editor/pickers/parse.ts` for the same reason: it is
 * testable without a renderer, so it is tested without one.
 */

import {useEffect, useMemo, useRef, useState} from "react"
import {StyleSheet, Text, View} from "react-native"

import {
    applyFieldEdit,
    ConfigEditCoalescer,
    isDrawableField,
} from "../../../layout/edit/panel.ts"
import {setWidgetConfig} from "../../../layout/edit/ops.ts"
import {BooleanPicker} from "./pickers/boolean"
import {ColorPicker} from "./pickers/color"
import {DatePicker} from "./pickers/date"
import {MultiSelectPicker} from "./pickers/multiselect"
import {NumberPicker} from "./pickers/number"
import {SelectPicker} from "./pickers/select"
import {TextPicker} from "./pickers/text"
import {UriPicker} from "./pickers/uri"

import type {ComponentType} from "react"

import type {EditContext, LayoutSocket} from "../../../layout/edit/ops.ts"
import type {FieldInput} from "../../../layout/edit/panel.ts"
import type {FieldDescriptor} from "../../../layout/registry/fields.ts"
import type {WidgetDefinition} from "../../../layout/registry/registry.ts"
import type {PickerProps} from "./pickers/props"

/**
 * The descriptor-kind to control table.
 *
 * A MAPPED TYPE OVER THE UNION, not a `Record<string, ...>` and not an index
 * signature. This is the enforcement mechanism that justifies hand-writing
 * field descriptors at all: adding a kind to `FieldDescriptor` without adding
 * a picker for it here fails to compile, so "descriptor kind nobody can draw"
 * is unrepresentable rather than a blank space in a panel somebody notices in
 * production. Weakening this type to a `Record` defeats the whole design.
 */
const PICKERS: {[K in FieldDescriptor["kind"]]: ComponentType<PickerProps>} = {
    text: TextPicker,
    number: NumberPicker,
    color: ColorPicker,
    boolean: BooleanPicker,
    date: DatePicker,
    select: SelectPicker,
    multiselect: MultiSelectPicker,
    uri: UriPicker,
}

export interface ConfigPanelProps {
    /** The selected widget's registry definition: its fields, schema and defaults. */
    def: WidgetDefinition
    /** The selected widget's key in its scene's widget map. */
    widgetId: string
    /** The widget's config as the SERVER last reported it — never a local draft. */
    config: Record<string, unknown>
    socket: LayoutSocket
    ctx: EditContext
}

/**
 * Draw the config panel for one widget.
 *
 * Nothing is applied optimistically, exactly as in `layout/edit/ops.ts`: the
 * `config` prop always reflects the last delta the server published, so the
 * editing host watches its own edit land the same way every other player does.
 * The only local state is the per-field error and the pending debounce.
 */
export function ConfigPanel({def, widgetId, config, socket, ctx}: ConfigPanelProps) {
    const [errors, setErrors] = useState<Record<string, string>>({})

    // The socket and edit context change identity across a reconnect or a scene
    // switch. The coalescer must NOT, or its timer would be discarded together
    // with whatever was half-typed, so the current send is reached through a ref
    // rather than through the coalescer's dependencies.
    const emit = useRef((patch: Record<string, unknown>) => {
        setWidgetConfig(socket, ctx, widgetId, patch)
    })

    useEffect(() => {
        emit.current = (patch) => setWidgetConfig(socket, ctx, widgetId, patch)
    }, [socket, ctx, widgetId])

    const coalescer = useMemo(() => new ConfigEditCoalescer((patch) => emit.current(patch)), [])

    // Flush on unmount: closing the panel must not swallow the last keystroke.
    useEffect(() => () => coalescer.flush(), [coalescer])

    function handleChange(field: FieldDescriptor, raw: FieldInput): void {
        const edit = applyFieldEdit(def, config, field, raw, coalescer)

        if (!edit.ok) {
            // Reported and NOT sent. The panel is the one place a human types an
            // arbitrary value, so it is the one place that has to refuse one.
            setErrors((prev) => ({...prev, [field.key]: edit.error}))

            return
        }

        setErrors((prev) => {
            if (prev[field.key] === undefined) return prev

            const next = {...prev}
            delete next[field.key]

            return next
        })
    }

    return (
        <View style={styles.panel}>
            {def.fields.map((field) => {
                const value = config[field.key] ?? def.defaults[field.key]
                if (!isDrawableField(field, value)) {
                    return <UndrawableField key={field.key} field={field} />
                }

                const Picker = PICKERS[field.kind]

                return (
                    <Picker
                        key={field.key}
                        field={field}
                        value={value}
                        error={errors[field.key]}
                        onChange={(raw) => handleChange(field, raw)}
                    />
                )
            })}
        </View>
    )
}

/**
 * Drawn in place of a field whose value no single input can edit.
 *
 * Shown rather than skipped: a config key that a panel silently omits is
 * indistinguishable from one that does not exist. See `isDrawableField` for
 * why this case exists at all.
 */
function UndrawableField({field}: {field: FieldDescriptor}) {
    return (
        <View style={styles.field}>
            <Text style={styles.label}>{field.label}</Text>
            <Text style={styles.note}>{field.description ?? "Edited on the canvas, not here."}</Text>
        </View>
    )
}

const styles = StyleSheet.create({
    panel: {
        padding: 8,
        gap: 8,
    },
    field: {
        gap: 2,
    },
    label: {
        fontSize: 12,
        fontWeight: "600",
        color: "#111111",
    },
    note: {
        fontSize: 11,
        fontStyle: "italic",
        color: "#6b7280",
    },
    error: {
        fontSize: 11,
        color: "#b91c1c",
    },
})
