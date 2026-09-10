import {StyleSheet, Text} from "react-native"

import {WidgetConfigError} from "../config-error"
import {TEXT_TYPE, TextConfigSchema, textDefinition} from "./definition"

import type {TextStyle} from "react-native"
import type {WidgetDefinition, WidgetRenderProps} from "@/layout/registry/registry"
import type {TextConfig} from "./definition"

/**
 * A run of text filling its widget box.
 *
 * `widget.config` is parsed here rather than handed over pre-parsed: see
 * `WidgetRenderProps`. No cast — a config that does not match the schema draws
 * the shared error box instead.
 */
export function TextWidgetView({id, widget}: WidgetRenderProps) {
    const parsed = TextConfigSchema.safeParse(widget.config ?? {})
    if (!parsed.success) {
        return <WidgetConfigError id={id} type={TEXT_TYPE} error={parsed.error}/>
    }

    return <Text style={[styles.text, textStyle(parsed.data)]}>{parsed.data.text}</Text>
}

/**
 * Only the keys the author actually set.
 *
 * An explicit `undefined` in an RN style object still overrides an earlier
 * entry in the array, so spreading the config wholesale would wipe out
 * `styles.text` for every key the author left out.
 */
function textStyle(config: TextConfig): TextStyle {
    return {
        ...(config.fontSize === undefined ? null : {fontSize: config.fontSize}),
        ...(config.color === undefined ? null : {color: config.color}),
        ...(config.align === undefined ? null : {textAlign: config.align}),
        ...(config.weight === undefined ? null : {fontWeight: config.weight}),
    }
}

/** The full definition: the config contract plus its renderer. */
export const TextWidget: WidgetDefinition<TextConfig> = {
    ...textDefinition,
    Component: TextWidgetView,
}

const styles = StyleSheet.create({
    // `flex: 1` so the text spans its widget box; without it the Text shrinks
    // to its content and `textAlign` has nothing to align within.
    text: {
        flex: 1,
    },
})
