import React from "react"
import {StyleSheet} from "react-native"
import {Image} from "expo-image"
import type {WidgetProps} from "@/layout/registry/registry.ts"

export type ImageConfig = {
    uri: string
    resizeMode?: "contain" | "cover" | "stretch" | "repeat" | "center"
}

// Map the schema's resizeMode enum to expo-image's contentFit prop.
// expo-image uses "contentFit" rather than "resizeMode". Most values map directly;
// "repeat" has no contentFit equivalent (expo-image documents it as unsupported),
// so we fall back to "contain" to avoid a blank box.
const CONTENT_FIT_MAP = {
    contain: "contain",
    cover: "cover",
    stretch: "fill",
    repeat: "contain",  // expo-image does not support tiling; "contain" is the safe fallback
    center: "none",
} as const satisfies Record<string, import("expo-image").ImageContentFit>

export function ImageWidget({config}: WidgetProps<ImageConfig>) {
    const contentFit = config.resizeMode !== undefined
        ? CONTENT_FIT_MAP[config.resizeMode]
        : "contain"

    return (
        <Image
            source={config.uri}
            contentFit={contentFit}
            style={styles.image}
            // expo-image shows its own placeholder automatically on load error —
            // no onError handler needed; never a blank box, never a throw.
        />
    )
}

const styles = StyleSheet.create({
    image: {
        flex: 1,
        width: "100%",
        height: "100%",
    },
})
