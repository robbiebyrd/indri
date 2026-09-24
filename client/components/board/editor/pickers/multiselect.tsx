import React from "react"
import {Text, View, Pressable, StyleSheet} from "react-native"
import {dedupeValues} from "@/layout/edit/picker-helpers"
import type {FieldDescriptor} from "@/layout/registry/fields"
import type {PickerProps} from "./text"

export function MultiSelectPicker({field, value, onChange}: PickerProps) {
    const f = field as Extract<FieldDescriptor, {kind: "multiselect"}>
    const current: string[] = Array.isArray(value) ? (value as string[]) : []

    function toggle(v: string) {
        const next = current.includes(v)
            ? current.filter(x => x !== v)
            : dedupeValues([...current, v])
        onChange(next)
    }

    return (
        <View style={styles.row}>
            <Text style={styles.label}>{f.label}</Text>
            {f.options.map(o => {
                const sv = String(o.value)
                const selected = current.includes(sv)
                return (
                    <Pressable
                        key={sv}
                        style={[styles.option, selected && styles.optionSelected]}
                        onPress={() => toggle(sv)}
                    >
                        <Text style={[styles.optionText, selected && styles.optionTextSelected]}>
                            {o.label}
                        </Text>
                    </Pressable>
                )
            })}
        </View>
    )
}

const styles = StyleSheet.create({
    row: {marginBottom: 10},
    label: {fontSize: 12, color: "#555", marginBottom: 4},
    option: {
        paddingVertical: 8,
        paddingHorizontal: 12,
        borderWidth: 1,
        borderColor: "#ccc",
        borderRadius: 4,
        marginBottom: 4,
    },
    optionSelected: {
        borderColor: "#2563eb",
        backgroundColor: "#e0ecff",
    },
    optionText: {color: "#222"},
    optionTextSelected: {color: "#1d4ed8", fontWeight: "600"},
})
