import React, {useState, useRef} from "react"
import {TextInput, Text, View, StyleSheet} from "react-native"
import {parseColor} from "@/layout/edit/picker-helpers"
import type {FieldDescriptor} from "@/layout/registry/fields"
import type {PickerProps} from "./text"

export function ColorPicker({field, value, onChange}: PickerProps) {
    const f = field as Extract<FieldDescriptor, {kind: "color"}>
    const [draft, setDraft] = useState(typeof value === "string" ? value : "")
    const [error, setError] = useState<string | null>(null)
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

    const swatch = parseColor(draft)

    function handleChange(text: string) {
        setDraft(text)
        const parsed = parseColor(text)
        if (parsed) {
            setError(null)
            if (timer.current) clearTimeout(timer.current)
            timer.current = setTimeout(() => onChange(parsed), 250)
        } else {
            setError("Invalid colour (use #rgb or #rrggbb)")
        }
    }

    return (
        <View style={styles.row}>
            <Text style={styles.label}>{f.label}</Text>
            <View style={styles.inputRow}>
                {swatch && (
                    <View style={[styles.swatch, {backgroundColor: swatch}]} />
                )}
                <TextInput
                    style={[styles.input, error ? styles.inputError : null]}
                    value={draft}
                    onChangeText={handleChange}
                    placeholder="#rrggbb"
                    placeholderTextColor="#888"
                    autoCapitalize="none"
                />
            </View>
            {error && <Text style={styles.errorText}>{error}</Text>}
        </View>
    )
}

const styles = StyleSheet.create({
    row: {marginBottom: 10},
    label: {fontSize: 12, color: "#555", marginBottom: 4},
    inputRow: {flexDirection: "row", alignItems: "center", gap: 8},
    swatch: {width: 24, height: 24, borderRadius: 4, borderWidth: 1, borderColor: "#ccc"},
    input: {
        flex: 1,
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
