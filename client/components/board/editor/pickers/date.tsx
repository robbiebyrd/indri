/** The `date` picker: three numeric boxes, committed as one ISO date. */

import {useState} from "react"
import {StyleSheet, Text, TextInput, View} from "react-native"

import {FieldShell, styles, TOUCH_TARGET} from "./common"
import {BLANK_DATE, isBlankDate, joinIsoDate, splitIsoDate, toInputText} from "./parse"

import type {DateParts} from "./parse"
import type {PickerProps} from "./props"

/**
 * WHAT THIS IS: segmented year / month / day entry over a numeric keypad. What
 * leaves it is always `YYYY-MM-DD` and always a date that exists — `joinIsoDate`
 * refuses 30 February rather than rolling it into March the way `new Date` does.
 * Clearing all three boxes clears the field.
 *
 * WHAT THIS IS NOT: a calendar. There is no cross-platform date primitive in
 * React Native and the brief forbids a library, so there is no month grid, no
 * "today", no locale-ordered segments (the order is ISO's, everywhere) and no
 * time of day. It is also not a range: one date, one config key.
 *
 * COMMITS ON BLUR. `date` is not a debounced kind, and a partially typed year
 * passes through states that are themselves valid dates — `2`, `20`, `202` are
 * years 2, 20 and 202 — so emitting per keystroke would publish three layout
 * ops nobody meant on the way to typing `2026`.
 */
export function DatePicker({field, value, error, onChange}: PickerProps) {
    const iso = toInputText(value)

    // The three boxes ARE the draft; there is nothing else to hold it in, since
    // a segment is only meaningful together with the other two.
    const [parts, setParts] = useState<DateParts>(() => splitIsoDate(iso) ?? BLANK_DATE)
    const [seen, setSeen] = useState(iso)

    if (iso !== seen) {
        setSeen(iso)
        setParts(splitIsoDate(iso) ?? BLANK_DATE)
    }

    const commit = () => {
        if (isBlankDate(parts)) {
            // Emptying every box is a deliberate "no date", not an incomplete
            // one, so it clears the key instead of reporting an error.
            if (iso !== "") onChange("")

            return
        }

        const next = joinIsoDate(parts)
        // An incomplete or impossible date is held in the boxes and explained
        // by `hint`; sending it would only produce the same complaint from
        // further away.
        if (next !== undefined && next !== iso) onChange(next)
    }

    const incomplete = !isBlankDate(parts) && joinIsoDate(parts) === undefined

    return (
        <FieldShell
            field={field}
            error={error}
            hint={incomplete ? "Enter a real date as year, month, day" : undefined}
        >
            <View style={styles.row}>
                <Segment
                    label="YYYY"
                    width={4}
                    value={parts.year}
                    onChangeText={(year) => setParts({...parts, year})}
                    onCommit={commit}
                />
                <Text style={dateStyles.separator}>-</Text>
                <Segment
                    label="MM"
                    width={2}
                    value={parts.month}
                    onChangeText={(month) => setParts({...parts, month})}
                    onCommit={commit}
                />
                <Text style={dateStyles.separator}>-</Text>
                <Segment
                    label="DD"
                    width={2}
                    value={parts.day}
                    onChangeText={(day) => setParts({...parts, day})}
                    onCommit={commit}
                />
            </View>
        </FieldShell>
    )
}

interface SegmentProps {
    label: string
    /** Digits this segment holds, which is also how wide it is drawn. */
    width: number
    value: string
    onChangeText: (text: string) => void
    onCommit: () => void
}

/**
 * Not `DraftInput`: that component owns its own draft, and these three have to
 * share one, because a year alone is not a date.
 */
function Segment({label, width, value, onChangeText, onCommit}: SegmentProps) {
    return (
        <TextInput
            value={value}
            placeholder={label}
            placeholderTextColor="#9ca3af"
            keyboardType="number-pad"
            maxLength={width}
            onChangeText={onChangeText}
            onBlur={onCommit}
            onSubmitEditing={onCommit}
            style={[styles.input, dateStyles.segment, {width: width * 12 + 24}]}
        />
    )
}

const dateStyles = StyleSheet.create({
    segment: {
        minHeight: TOUCH_TARGET,
        textAlign: 'center',
        paddingHorizontal: 4,
    },
    separator: {
        color: '#6b7280',
    },
})
