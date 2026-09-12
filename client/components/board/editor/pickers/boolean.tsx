/** The `boolean` picker: one 44pt row that toggles on tap. */

// The named export, not the default: they are the same component, and eslint's
// `import/no-named-as-default` flags shadowing a named export with a default.
import {Checkbox} from "expo-checkbox"
import {Pressable, StyleSheet, Text, View} from "react-native"

import {FieldShell, styles, TOUCH_TARGET} from "./common"

import type {PickerProps} from "./props"

/**
 * `expo-checkbox` because it is already a dependency of this app and already
 * cross-platform — a hand-rolled box would be a third thing to keep looking
 * right on three platforms for no gain.
 *
 * THE WHOLE ROW IS THE TARGET, not the box. A checkbox glyph is well under
 * 44pt on every platform, so the `Pressable` carries the gesture and the
 * checkbox is made non-interactive rather than wired up twice — two handlers on
 * one tap would toggle and immediately untoggle.
 *
 * A non-boolean value reads as `false` rather than refusing to draw: the panel
 * has already established the value is a primitive, and an unchecked box a host
 * can tick is a way out of a bad config. Ticking it sends a real `true`.
 */
export function BooleanPicker({field, value, error, onChange}: PickerProps) {
    const checked = value === true

    return (
        <FieldShell field={field} error={error}>
            <Pressable
                style={[styles.row, toggleStyles.row]}
                onPress={() => onChange(!checked)}
                accessibilityRole="checkbox"
                accessibilityState={{checked}}
            >
                <View pointerEvents="none">
                    <Checkbox value={checked} color={checked ? '#2563eb' : undefined}/>
                </View>
                <Text style={toggleStyles.state}>{checked ? "On" : "Off"}</Text>
            </Pressable>
        </FieldShell>
    )
}

const toggleStyles = StyleSheet.create({
    row: {
        minHeight: TOUCH_TARGET,
        paddingHorizontal: 12,
        borderWidth: 1,
        borderColor: '#d4d4d8',
        borderRadius: 4,
        backgroundColor: '#ffffff',
    },
    state: {
        color: '#18181b',
    },
})
