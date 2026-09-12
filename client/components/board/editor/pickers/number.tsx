/** The `number` picker: a numeric box between two steppers. */

import {useState} from "react"
import {Pressable, Text, View} from "react-native"

import {DraftInput, FieldShell, styles} from "./common"
import {numberRange, parseNumberText, stepNumber, toInputText} from "./parse"

import type {PickerProps} from "./props"

/**
 * TYPED TEXT AND A STEPPER TAP ARE DELIBERATELY DIFFERENT. Typing is an
 * assertion about a value, so `32768` in a box capped at 512 is sent, rejected
 * by `coerceFieldValue` and reported. A tap is not an assertion, so `stepNumber`
 * clamps and the button simply stops at the bound — see `parse.ts`.
 *
 * THE ONE PIECE OF LOCAL STATE IN ANY PICKER, and why it has to exist: `number`
 * is debounced, so five quick taps on `+` produce five edits inside one 250ms
 * window, and the panel's `value` prop does not move until the op lands. Each
 * tap computed from `value` alone would therefore be `value + 1` five times
 * over — a stepper that counts to one. `stepped` is the running total those
 * taps accumulate on, and it is discarded the moment the server publishes
 * anything, which is the only thing that makes it non-authoritative.
 */
export function NumberPicker({field, value, error, onChange}: PickerProps) {
    const range = numberRange(field)
    const text = toInputText(value)

    const [stepped, setStepped] = useState<number | undefined>(undefined)
    const [seen, setSeen] = useState(text)

    if (text !== seen) {
        setSeen(text)
        setStepped(undefined)
    }

    const current = stepped ?? parseNumberText(text)

    const bump = (direction: 1 | -1) => {
        // An empty or unparseable box has nothing to step from, so the first
        // tap seeds it at the lower bound rather than doing nothing at all.
        const next = current === undefined
            ? range.min ?? 0
            : stepNumber(range, current, direction)

        setStepped(next)
        onChange(next)
    }

    return (
        <FieldShell field={field} error={error} hint={bounds(range.min, range.max)}>
            <View style={styles.row}>
                <Stepper label="−" onPress={() => bump(-1)}/>
                <DraftInput
                    value={stepped === undefined ? text : String(stepped)}
                    keyboardType="numeric"
                    onEdit={(typed) => {
                        // Typing overrides the stepper's running total; the box
                        // is now saying what the value is.
                        setStepped(undefined)
                        onChange(typed)
                    }}
                    style={styles.grow}
                />
                <Stepper label="+" onPress={() => bump(1)}/>
            </View>
        </FieldShell>
    )
}

function Stepper({label, onPress}: {label: string; onPress: () => void}) {
    return (
        <Pressable style={styles.button} onPress={onPress}>
            <Text style={styles.buttonText}>{label}</Text>
        </Pressable>
    )
}

/** The bounds, said once under the box, so a rejection is never a surprise. */
function bounds(min?: number, max?: number): string | undefined {
    if (min !== undefined && max !== undefined) return `${min} to ${max}`
    if (min !== undefined) return `${min} or more`
    if (max !== undefined) return `${max} or less`

    return undefined
}
