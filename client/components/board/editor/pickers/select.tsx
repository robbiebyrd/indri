import React from "react"
import {Text, View, StyleSheet} from "react-native"
import SelectControl from "@/components/display/select"
import type {FieldDescriptor} from "@/layout/registry/fields"
import type {PickerProps} from "./text"

export function SelectPicker({field, value, onChange}: PickerProps) {
    const f = field as Extract<FieldDescriptor, {kind: "select"}>
    const options = f.options.map(o => ({value: String(o.value), label: o.label}))
    return (
        <View style={styles.row}>
            <Text style={styles.label}>{f.label}</Text>
            <SelectControl
                options={options}
                value={typeof value === "string" ? value : undefined}
                onChange={v => onChange(v)}
            />
        </View>
    )
}

const styles = StyleSheet.create({
    row: {marginBottom: 10},
    label: {fontSize: 12, color: "#555", marginBottom: 4},
})
