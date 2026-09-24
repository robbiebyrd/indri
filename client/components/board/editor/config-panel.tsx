import React from "react"
import {ScrollView, Text, StyleSheet} from "react-native"
import type {WidgetDefinition} from "@/layout/registry/registry"
import type {FieldDescriptor} from "@/layout/registry/fields"
import type {PickerProps} from "./pickers/text"
import {TextPicker} from "./pickers/text"
import {NumberPicker} from "./pickers/number"
import {ColorPicker} from "./pickers/color"
import {BooleanPicker} from "./pickers/boolean"
import {DatePicker} from "./pickers/date"
import {SelectPicker} from "./pickers/select"
import {MultiSelectPicker} from "./pickers/multiselect"
import {UriPicker} from "./pickers/uri"

// Mapped type: a new FieldDescriptor kind with no entry here is a compile error.
const PICKERS: {[K in FieldDescriptor["kind"]]: React.ComponentType<PickerProps>} = {
    text: TextPicker,
    number: NumberPicker,
    color: ColorPicker,
    boolean: BooleanPicker,
    date: DatePicker,
    select: SelectPicker,
    multiselect: MultiSelectPicker,
    uri: UriPicker,
}

type Props = {
    def: WidgetDefinition
    value: Record<string, unknown>
    onChange: (key: string, v: unknown) => void
}

/**
 * Renders a form for the widget's config fields using the PICKERS registry.
 * Only calls onChange when the new value passes the widget's Zod schema.
 */
export function ConfigPanel({def, value, onChange}: Props) {
    function handleChange(key: string, v: unknown) {
        const merged = {...value, [key]: v}
        const result = def.schema.safeParse(merged)
        if (!result.success) return // invalid — do not emit
        onChange(key, v)
    }

    if (def.fields.length === 0) {
        return <Text style={styles.empty}>No configurable fields</Text>
    }

    return (
        <ScrollView style={styles.scroll} contentContainerStyle={styles.content}>
            {def.fields.map(f => {
                const P = PICKERS[f.kind]
                return (
                    <P
                        key={f.key}
                        field={f}
                        value={value[f.key]}
                        onChange={v => handleChange(f.key, v)}
                    />
                )
            })}
        </ScrollView>
    )
}

const styles = StyleSheet.create({
    scroll: {maxHeight: 300},
    content: {padding: 12},
    empty: {color: "#888", fontSize: 12, padding: 12},
})
