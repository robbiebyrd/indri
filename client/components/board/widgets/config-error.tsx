import {StyleSheet, Text, View} from "react-native"

import type {ZodError} from "zod"

export interface WidgetConfigErrorProps {
    /** The widget's key in its parent's widget map. */
    id: string
    /** The widget type whose schema rejected the config. */
    type: string
    error: ZodError
}

/**
 * What a widget draws when its own config does not parse.
 *
 * Every renderer needs this, because `WidgetRenderProps` hands over the raw
 * `Widget` rather than a pre-parsed config: the registry stores erased
 * definitions and `ComponentType<P>` is contravariant in `P`, so a component
 * typed on a narrow config could not be stored in that map at all. Each
 * renderer therefore parses for itself, and each one can fail.
 *
 * VISIBLE rather than dev-only, and deliberately shaped like
 * `widget-host.tsx`'s missing-renderer placeholder. `parseLayout` validates a
 * widget's STRUCTURE and leaves `config` opaque, so a config that disagrees
 * with its widget type produces no layout issue at all — this box is the only
 * signal that anything is wrong, and rendering nothing would be
 * indistinguishable from an intentionally empty widget.
 */
export function WidgetConfigError({id, type, error}: WidgetConfigErrorProps) {
    return (
        <View style={styles.box}>
            <Text style={styles.text} numberOfLines={4}>
                {`bad "${type}" config (${id}): ${summarise(error)}`}
            </Text>
        </View>
    )
}

function summarise(error: ZodError): string {
    return error.issues
        .map((issue) => `${issue.path.join(".") || "<root>"}: ${issue.message}`)
        .join("; ")
}

const styles = StyleSheet.create({
    box: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
        padding: 4,
        borderWidth: 1,
        borderStyle: 'dashed',
        borderColor: '#b91c1c',
        backgroundColor: '#fee2e2',
    },
    text: {
        color: '#7f1d1d',
        fontSize: 10,
        textAlign: 'center',
    },
})
