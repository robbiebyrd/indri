import {Image} from "expo-image"
import {StyleSheet, View} from "react-native"

import {contentFitFor} from "../../layers"
import {WidgetConfigError} from "../config-error"
import {DEFAULT_RESIZE_MODE, IMAGE_TYPE, ImageConfigSchema, imageDefinition} from "./definition"

import type {WidgetDefinition, WidgetRenderProps} from "@/layout/registry/registry"
import type {ImageConfig} from "./definition"

/**
 * A picture filling its widget box.
 *
 * `expo-image` covers png, jpg, gif, webp, avif and svg on web, iOS and
 * Android, which is why no `react-native-svg` dependency is needed.
 *
 * FAILURE BEHAVIOUR. `expo-image` reports a source it cannot fetch through
 * `onError` and renders nothing; it never throws, so a bad URI cannot take out
 * the board. What it renders instead is the container beneath it, which is
 * given a neutral fill on purpose — that fill is the placeholder, and it is
 * painted before the fetch starts, so a slow URI and a broken one look the
 * same and neither ever leaves a blank hole in the layout.
 */
export function ImageWidgetView({id, widget}: WidgetRenderProps) {
    const parsed = ImageConfigSchema.safeParse(widget.config ?? {})
    if (!parsed.success) {
        return <WidgetConfigError id={id} type={IMAGE_TYPE} error={parsed.error}/>
    }

    const {uri, resizeMode, alt} = parsed.data

    return (
        <View style={styles.container}>
            <Image
                // `null` rather than an empty uri: a widget with no source yet
                // should show the placeholder, not start a doomed fetch.
                source={uri === "" ? null : uri}
                contentFit={contentFitFor(resizeMode ?? DEFAULT_RESIZE_MODE)}
                alt={alt}
                style={StyleSheet.absoluteFill}
            />
        </View>
    )
}

/** The full definition: the config contract plus its renderer. */
export const ImageWidget: WidgetDefinition<ImageConfig> = {
    ...imageDefinition,
    Component: ImageWidgetView,
}

const styles = StyleSheet.create({
    // The placeholder. Visible whenever the image has not painted over it —
    // while loading, when the source is unset, and when the fetch fails.
    // `overflow: hidden` keeps a `contentFit` that overspills inside the box.
    container: {
        flex: 1,
        overflow: 'hidden',
        backgroundColor: '#e5e7eb',
    },
})
