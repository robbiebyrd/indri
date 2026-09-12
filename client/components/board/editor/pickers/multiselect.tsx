/** The `multiselect` picker: the same list of choices, more than one lit. */

import {MultiSelect} from "@/components/display/select"

import {FieldShell} from "./common"
import {fieldOptions, selectedValues, toggleValue} from "./parse"

import type {PickerProps} from "./props"

/**
 * ORDER IS THE SELECTION'S, NOT THE OPTION LIST'S. `selectedValues` keeps the
 * stored order and `toggleValue` appends rather than inserting, so a re-render
 * cannot reshuffle a selection and a single tap cannot publish a delta full of
 * moves nobody made. Both live in `./parse.ts` and are tested there — this
 * component only wires them to the rows.
 *
 * `MultiSelect` reports which row was pressed and nothing more, deliberately:
 * it is a generic display control and has no business owning the ordering rules
 * of a layout config value.
 */
export function MultiSelectPicker({field, value, error, onChange}: PickerProps) {
    const options = fieldOptions(field)
    const values = selectedValues(value, options)

    return (
        <FieldShell field={field} error={error}>
            <MultiSelect
                options={options}
                values={values}
                placeholder="This field offers no choices"
                onToggle={(chosen) => onChange(toggleValue(values, chosen))}
            />
        </FieldShell>
    )
}
