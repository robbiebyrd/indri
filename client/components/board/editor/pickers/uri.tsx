import React, {useState, useRef} from "react"
import {TextInput, Text, View, StyleSheet} from "react-native"
import type {FieldDescriptor} from "@/layout/registry/fields"
import type {PickerProps} from "./text"

export function UriPicker({field, value, onChange}: PickerProps) {
    const f = field as Extract<FieldDescriptor, {kind: "uri"}>
    const [draft, setDraft] = useState(typeof value === "string" ? value : "")
    const [error, setError] = useState<string | null>(null)
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

    function handleChange(text: string) {
        setDraft(text)
        try {
            new URL(text)
            setError(null)
            if (timer.current) clearTimeout(timer.current)
            timer.current = setTimeout(() => onChange(text), 250)
        } catch {
            setError(`Enter a valid ${f.accept} URL`)
        }
    }

    return (
        <View style={styles.row}>
            <Text style={styles.label}>{f.label}</Text>
            <TextInput
                style={[styles.input, error ? styles.inputError : null]}
                value={draft}
                onChangeText={handleChange}
                placeholder={`https://example.com/${f.accept}.png`}
                placeholderTextColor="#888"
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="url"
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
