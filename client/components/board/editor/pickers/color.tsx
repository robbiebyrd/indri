/** The `color` picker: a hex box with a live swatch beside it. */

import {useState} from "react"
import {StyleSheet, View} from "react-native"

import {DraftInput, FieldShell, styles, TOUCH_TARGET} from "./common"
import {normalizeHexColor, toInputText} from "./parse"

import type {PickerProps} from "./props"

/**
 * WHAT THIS IS: a text field that understands hex, with a swatch that updates
 * as you type. `#ABC` becomes `#aabbcc` when it is committed, so two spellings
 * of one colour do not read as two colours in a diff.
 *
 * WHAT THIS IS NOT: a colour wheel, an eyedropper, or an alpha slider. React
 * Native has no cross-platform colour primitive and the brief forbids adding a
 * library for one, so the honest limited control wins over a rich broken one.
 * Anything the platform accepts but hex cannot express — `red`, `rgba(...)`,
 * `hsl(...)` — is passed through UNCHANGED and simply draws no swatch, because
 * the widget schema is a plain string for exactly that reason. A blank swatch
 * therefore means "not previewable here", never "not a colour".
 *
 * COMMITS ON BLUR, not per keystroke. `color` is not one of the debounced
 * kinds, so a per-keystroke edit would be one layout op — and one full state
 * replay on every device in the game — per character of `#aabbcc`.
 */
export function ColorPicker({field, value, error, onChange}: PickerProps) {
    const text = toInputText(value)

    // The swatch previews what is BEING typed, which the committed value does
    // not know about yet. Reset whenever the server publishes something.
    const [typing, setTyping] = useState(text)
    const [seen, setSeen] = useState(text)

    if (text !== seen) {
        setSeen(text)
        setTyping(text)
    }

    const hex = normalizeHexColor(typing)

    return (
        <FieldShell
            field={field}
            error={error}
            hint={hex === undefined && typing.trim() !== "" ? "Not a hex colour — no preview" : undefined}
        >
            <View style={styles.row}>
                <View
                    style={[
                        swatchStyles.swatch,
                        // Only ever a value `normalizeHexColor` vouched for: an
                        // unparseable string handed to `backgroundColor` is a
                        // hard error on native, not a shrug.
                        hex === undefined ? swatchStyles.unknown : {backgroundColor: hex},
                    ]}
                />
                <DraftInput
                    value={text}
                    placeholder="#rrggbb"
                    style={styles.grow}
                    onEdit={setTyping}
                    onCommit={(typed) => onChange(normalizeHexColor(typed) ?? typed.trim())}
                />
            </View>
        </FieldShell>
    )
}

const swatchStyles = StyleSheet.create({
    swatch: {
        width: TOUCH_TARGET,
        height: TOUCH_TARGET,
        borderWidth: 1,
        borderColor: '#d4d4d8',
        borderRadius: 4,
    },
    // Read as "nothing to show", not as the colour white.
    unknown: {
        backgroundColor: '#ffffff',
        borderStyle: 'dashed',
    },
})
