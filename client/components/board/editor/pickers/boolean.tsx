import React from "react"
import {Switch, Text, View, StyleSheet} from "react-native"
import type {FieldDescriptor} from "@/layout/registry/fields"
import type {PickerProps} from "./text"

export function BooleanPicker({field, value, onChange}: PickerProps) {
    const f = field as Extract<FieldDescriptor, {kind: "boolean"}>
    return (
        <View style={styles.row}>
            <Text style={styles.label}>{f.label}</Text>
            <Switch
                value={Boolean(value)}
                onValueChange={v => onChange(v)}
            />
        </View>
    )
}

const styles = StyleSheet.create({
    row: {flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: 10},
    label: {fontSize: 12, color: "#555"},
})
