/** The `select` picker: the app's existing dependency-free list of choices. */

import Select from "@/components/display/select"

import {FieldShell} from "./common"
import {fieldOptions} from "./parse"

import type {PickerProps} from "./props"

/**
 * `components/display/select.tsx` rather than a picker library: it already
 * exists precisely because `react-select` is DOM-only and breaks native builds,
 * and its rows already meet the 44pt touch target.
 *
 * Every row emits immediately — `select` is not a debounced kind, and one tap
 * is one whole value, with nothing to coalesce.
 */
export function SelectPicker({field, value, error, onChange}: PickerProps) {
    return (
        <FieldShell field={field} error={error}>
            <Select
                options={fieldOptions(field)}
                value={typeof value === "string" ? value : undefined}
                placeholder="This field offers no choices"
                onChange={onChange}
            />
        </FieldShell>
    )
}
