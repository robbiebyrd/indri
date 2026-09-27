import React, {useState, useRef} from "react"
import {TextInput, Text, View, StyleSheet} from "react-native"
import {clampNumber} from "@/layout/edit/picker-helpers"
import type {FieldDescriptor} from "@/layout/registry/fields"
import type {PickerProps} from "./text"

export function NumberPicker({field, value, onChange}: PickerProps) {
    const f = field as Extract<FieldDescriptor, {kind: "number"}>
    const [draft, setDraft] = useState(String(typeof value === "number" ? value : ""))
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

    function handleChange(text: string) {
        setDraft(text)
        const n = parseFloat(text)
        if (!isNaN(n)) {
            const clamped = clampNumber(n, f.min, f.max)
            if (timer.current) clearTimeout(timer.current)
            timer.current = setTimeout(() => onChange(clamped), 250)
        }
    }

    return (
        <View style={styles.row}>
            <Text style={styles.label}>{f.label}</Text>
            <TextInput
                style={styles.input}
                value={draft}
                onChangeText={handleChange}
                keyboardType="numeric"
                placeholder={f.label}
                placeholderTextColor="#888"
            />
        </View>
    )
}

const styles = StyleSheet.create({
    row: {marginBottom: 10},
    label: {fontSize: 12, color: "#555", marginBottom: 4},
    input: {
        borderWidth: 1,
        borderColor: "#ccc",
        borderRadius: 4,
        paddingHorizontal: 8,
        paddingVertical: 6,
        fontSize: 14,
        color: "#222",
    },
})
