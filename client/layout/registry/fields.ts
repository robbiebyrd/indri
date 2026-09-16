type FieldBase = {
    key: string       // matches the config schema key
    label: string     // human-readable label for the config panel
    required?: boolean
}

type Option = {value: string | number; label: string}

export type FieldDescriptor = FieldBase & (
    | {kind: "text"; multiline?: boolean}
    | {kind: "number"; min?: number; max?: number; step?: number}
    | {kind: "color"}
    | {kind: "boolean"}
    | {kind: "date"}
    | {kind: "select"; options: Option[]}
    | {kind: "multiselect"; options: Option[]}
    | {kind: "uri"; accept: "image" | "video"}
)
