/** The `text` picker: a one-line or multi-line box, debounced as it is typed. */

import {DraftInput, FieldShell} from "./common"
import {textLimits, toInputText} from "./parse"

import type {PickerProps} from "./props"

/**
 * `onEdit` rather than `onCommit` — `text` is one of the two kinds
 * `isDebouncedKind` covers, so per-keystroke edits are collapsed into one
 * layout op by the coalescer and a value is never lost to a forgotten blur.
 *
 * `maxLength` is NOT passed to the input. Capping it there would silently
 * truncate a paste; showing a counter and letting `coerceFieldValue` report the
 * overrun tells the author what happened.
 */
export function TextPicker({field, value, error, onChange}: PickerProps) {
    const {multiline, maxLength} = textLimits(field)
    const text = toInputText(value)
    const hint = maxLength === undefined ? undefined : `${text.length} / ${maxLength}`

    return (
        <FieldShell field={field} error={error} hint={hint}>
            <DraftInput value={text} multiline={multiline} prose onEdit={onChange}/>
        </FieldShell>
    )
}
