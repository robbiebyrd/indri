/**
 * What every picker is handed.
 *
 * Its own module rather than a declaration inside `config-panel.tsx`, so the
 * pickers and the table that dispatches to them both depend on this instead of
 * on each other. It is also pure TypeScript — no react, no react-native — which
 * keeps it loadable by anything that needs to talk about a picker's contract.
 */

import type {FieldInput} from "../../../../layout/edit/panel.ts"
import type {FieldDescriptor} from "../../../../layout/registry/fields.ts"

/**
 * `field` is the WHOLE descriptor union, not the variant matching the picker.
 *
 * Narrowing it per kind would make `PICKERS` a table of mutually incompatible
 * component types, and `PICKERS[field.kind]` would then demand props satisfying
 * every kind at once — a type no descriptor can have. So the union stays, and
 * the four pickers that need kind-specific properties read them through the
 * accessors in `./parse.ts`, where the narrowing happens once and is tested.
 */
export interface PickerProps {
    field: FieldDescriptor
    /** The current config value, falling back to the widget's default. */
    value: unknown
    /** Why the last thing entered here was rejected, if it was. */
    error?: string
    /**
     * Hand over the RAW value. `applyFieldEdit` coerces it and runs the
     * widget's schema; a picker that pre-judged the value would be a second,
     * divergent authority on what the config may contain.
     */
    onChange: (raw: FieldInput) => void
}
