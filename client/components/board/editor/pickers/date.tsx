import React, {useState, useRef} from "react"
import {TextInput, Text, View, StyleSheet} from "react-native"
import {parseDate} from "@/layout/edit/picker-helpers"
import type {FieldDescriptor} from "@/layout/registry/fields"
import type {PickerProps} from "./text"

export function DatePicker({field, value, onChange}: PickerProps) {
    const f = field as Extract<FieldDescriptor, {kind: "date"}>
    const [draft, setDraft] = useState(typeof value === "string" ? value : "")
    const [error, setError] = useState<string | null>(null)
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

    function handleChange(text: string) {
        setDraft(text)
        const parsed = parseDate(text)
        if (parsed) {
            setError(null)
            if (timer.current) clearTimeout(timer.current)
            timer.current = setTimeout(() => onChange(parsed), 250)
        } else {
            setError("Use YYYY-MM-DD format")
        }
    }

    return (
        <View style={styles.row}>
            <Text style={styles.label}>{f.label}</Text>
            <TextInput
                style={[styles.input, error ? styles.inputError : null]}
                value={draft}
                onChangeText={handleChange}
                placeholder="YYYY-MM-DD"
                placeholderTextColor="#888"
                keyboardType="numbers-and-punctuation"
            />
            {error && <Text style={styles.errorText}>{error}</Text>}
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
    inputError: {borderColor: "#dc2626"},
    errorText: {fontSize: 11, color: "#dc2626", marginTop: 2},
})
