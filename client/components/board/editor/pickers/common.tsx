/**
 * The frame every picker draws inside, and the text box most of them use.
 *
 * Extracted so the eight picker files hold nothing but the control that is
 * actually different between them. The label, the required marker, the
 * descriptor's own description and the panel's error message are identical for
 * all eight, and eight copies of them would drift.
 */

import {useState} from "react"
import {StyleSheet, Text, TextInput, View} from "react-native"

import type {ReactNode} from "react"
import type {KeyboardTypeOptions, StyleProp, TextStyle} from "react-native"

import type {FieldDescriptor} from "../../../../layout/registry/fields.ts"

/** Minimum touch target, per the guidelines both iOS and Android publish. */
export const TOUCH_TARGET = 44

export interface FieldShellProps {
    field: FieldDescriptor
    /** The panel's rejection of the last edit. */
    error?: string
    /** The picker's own note about what it can and cannot do. */
    hint?: string
    children: ReactNode
}

/** Label, description, control, hint, error — in that order, for every kind. */
export function FieldShell({field, error, hint, children}: FieldShellProps) {
    return (
        <View style={styles.field}>
            <Text style={styles.label}>{field.required ? `${field.label} *` : field.label}</Text>
            {field.description === undefined ? null : (
                <Text style={styles.note}>{field.description}</Text>
            )}
            {children}
            {hint === undefined ? null : <Text style={styles.note}>{hint}</Text>}
            {error === undefined ? null : <Text style={styles.error}>{error}</Text>}
        </View>
    )
}

export interface DraftInputProps {
    /** The value as the SERVER last reported it. Nothing here is optimistic. */
    value: string
    /**
     * Every keystroke. Wire `onChange` to it only for the kinds
     * `isDebouncedKind` covers — for any other kind this fires an immediate
     * layout op per character typed.
     */
    onEdit?: (text: string) => void
    /** Blur and submit: the "deliberate gesture" a non-debounced kind waits for. */
    onCommit?: (text: string) => void
    placeholder?: string
    multiline?: boolean
    keyboardType?: KeyboardTypeOptions
    /**
     * Default off, because most of these boxes hold machine values — a hex
     * colour autocorrected to a word is not a colour. Prose opts back in.
     */
    prose?: boolean
    style?: StyleProp<TextStyle>
}

/**
 * A text box that holds what is being typed without fighting the server.
 *
 * THE PROBLEM THIS SOLVES. `value` is the last state the server published, and
 * the panel deliberately applies nothing optimistically. A plain controlled
 * `TextInput` bound straight to it would therefore snap back to the old text
 * on every keystroke until the delta returned — and with the 250ms debounce in
 * front of it, that is every keystroke.
 *
 * THE RULE. A changed `value` is adopted only while the box is NOT focused. A
 * change that arrives mid-typing is recorded and dropped, so another host's
 * edit cannot move the caret and, more importantly, so the box does not revert
 * for a moment on blur while the debounced edit is still in flight.
 */
export function DraftInput({
    value,
    onEdit,
    onCommit,
    placeholder,
    multiline,
    keyboardType,
    prose,
    style,
}: DraftInputProps) {
    const [focused, setFocused] = useState(false)
    const [seen, setSeen] = useState(value)
    const [draft, setDraft] = useState(value)

    // Adjusting state during render rather than in an effect: this is the
    // "derive state from props" case React documents, and an effect would draw
    // the stale text for one frame first.
    if (value !== seen) {
        setSeen(value)
        if (!focused) setDraft(value)
    }

    return (
        <TextInput
            value={draft}
            placeholder={placeholder}
            placeholderTextColor="#9ca3af"
            multiline={multiline}
            keyboardType={keyboardType}
            autoCapitalize={prose === true ? "sentences" : "none"}
            autoCorrect={prose === true}
            onFocus={() => setFocused(true)}
            onBlur={() => {
                setFocused(false)
                onCommit?.(draft)
            }}
            onSubmitEditing={() => onCommit?.(draft)}
            onChangeText={(text) => {
                setDraft(text)
                onEdit?.(text)
            }}
            style={[styles.input, multiline === true && styles.multiline, style]}
        />
    )
}

export const styles = StyleSheet.create({
    field: {
        gap: 2,
    },
    label: {
        fontSize: 12,
        fontWeight: '600',
        color: '#111111',
    },
    note: {
        fontSize: 11,
        fontStyle: 'italic',
        color: '#6b7280',
    },
    error: {
        fontSize: 11,
        color: '#b91c1c',
    },
    // 44pt tall even for a single line: a text box is a touch target too, and
    // the default RN height on Android is well under it.
    input: {
        minHeight: TOUCH_TARGET,
        paddingVertical: 8,
        paddingHorizontal: 12,
        borderWidth: 1,
        borderColor: '#d4d4d8',
        borderRadius: 4,
        backgroundColor: '#ffffff',
        color: '#18181b',
    },
    multiline: {
        minHeight: TOUCH_TARGET * 2,
        textAlignVertical: 'top',
    },
    // The horizontal arrangement of a control and the buttons beside it.
    row: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
    },
    grow: {
        flex: 1,
    },
    button: {
        minWidth: TOUCH_TARGET,
        minHeight: TOUCH_TARGET,
        alignItems: 'center',
        justifyContent: 'center',
        borderWidth: 1,
        borderColor: '#d4d4d8',
        borderRadius: 4,
        backgroundColor: '#f4f4f5',
    },
    buttonText: {
        fontSize: 18,
        color: '#18181b',
    },
})
