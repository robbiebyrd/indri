/** The `uri` picker: a plain address box, committed on blur. */

import {DraftInput, FieldShell} from "./common"
import {toInputText, uriAccept} from "./parse"

import type {PickerProps} from "./props"

/**
 * WHAT THIS IS: a text box for an address, with the accepted media named under
 * it. `accept` is a HINT to the author, not a filter — nothing here inspects
 * what is at the other end of the URI.
 *
 * WHAT THIS IS NOT: a file browser, a media library picker, or an uploader.
 * There is no asset store in this project to browse and no upload endpoint to
 * post to, so a "pick a file" button would have nowhere to put the file.
 *
 * NO VALIDATION, deliberately, and the widget schemas agree: `ImageConfig.uri`
 * is `z.string()` rather than `.url()` because `data:` URIs and relative asset
 * paths are legitimate sources that `.url()` rejects. A URI that does not
 * resolve is the renderer's problem — the image widget draws its alt text.
 *
 * COMMITS ON BLUR, since `uri` is not one of the debounced kinds and an address
 * typed a character at a time would be an address published a character at a
 * time.
 */
export function UriPicker({field, value, error, onChange}: PickerProps) {
    const accept = uriAccept(field)

    return (
        <FieldShell
            field={field}
            error={error}
            hint={accept === undefined ? undefined : `Address of ${accept === "image" ? "an image" : "a video"}`}
        >
            <DraftInput
                value={toInputText(value)}
                placeholder="https://…"
                keyboardType="url"
                onCommit={(typed) => onChange(typed.trim())}
            />
        </FieldShell>
    )
}
