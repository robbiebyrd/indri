import React, {useState, useRef} from "react"
import {TextInput, Text, View, StyleSheet} from "react-native"
import type {FieldDescriptor} from "@/layout/registry/fields"

export type PickerProps = {
    field: FieldDescriptor
    value: unknown
    onChange: (value: unknown) => void
}

export function TextPicker({field, value, onChange}: PickerProps) {
    const f = field as Extract<FieldDescriptor, {kind: "text"}>
    const [draft, setDraft] = useState(typeof value === "string" ? value : "")
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

    function handleChange(text: string) {
        setDraft(text)
        if (timer.current) clearTimeout(timer.current)
        timer.current = setTimeout(() => onChange(text), 250)
    }

    return (
        <View style={styles.row}>
            <Text style={styles.label}>{f.label}</Text>
            <TextInput
                style={styles.input}
                value={draft}
                onChangeText={handleChange}
                multiline={f.multiline}
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
