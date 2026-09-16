import React from "react"
import {View, StyleSheet} from "react-native"
import {Image} from "expo-image"
import type {ImageContentFit} from "expo-image"
import {LinearGradient} from "expo-linear-gradient"
import {compileStyle} from "@/layout/style/compile"
import type {Style, ResizeModeType} from "@/layout/schema/style"
import type {ReactNode} from "react"

// expo-image uses 'fill' where the style schema uses 'stretch', and has no
// direct equivalent for 'repeat' or 'center' — map to the closest fit.
const RESIZE_MODE_TO_CONTENT_FIT: Record<ResizeModeType, ImageContentFit> = {
    contain: "contain",
    cover: "cover",
    stretch: "fill",
    repeat: "cover",
    center: "none",
}

interface Props {
    style?: Style
    viewStyle?: object   // additional RN style (position, width, height from placement)
    children?: ReactNode
}

export function StyledBox({style, viewStyle, children}: Props) {
    const {viewStyle: compiledStyle, layers} = compileStyle(style)
    return (
        <View style={[compiledStyle, viewStyle]}>
            {layers.map((layer, i) =>
                layer.kind === "gradient" ? (
                    <LinearGradient
                        key={i}
                        colors={layer.colors as [string, string, ...string[]]}
                        locations={layer.locations as [number, number, ...number[]] | undefined}
                        start={layer.start}
                        end={layer.end}
                        style={StyleSheet.absoluteFill}
                    />
                ) : (
                    <Image
                        key={i}
                        source={{uri: layer.uri}}
                        contentFit={layer.resizeMode !== undefined ? RESIZE_MODE_TO_CONTENT_FIT[layer.resizeMode] : "cover"}
                        style={StyleSheet.absoluteFill}
                    />
                )
            )}
            {children}
        </View>
    )
}
