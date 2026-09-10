import {Image} from "expo-image"
import {LinearGradient} from "expo-linear-gradient"
import {StyleSheet, View} from "react-native"

import {compileStyle} from "@/layout/style/compile"
import {contentFitFor, gradientColors, gradientLocations} from "./layers"

import type {ReactNode} from "react"
import type {StyleProp, ViewStyle} from "react-native"
import type {Style} from "@/layout/schema/style"

export interface StyledBoxProps {
    /** Authored style off the wire. Compiled to RN props plus background layers. */
    style?: Style
    /**
     * Structural RN style — position, the percentage box, stacking order.
     * Applied AFTER the compiled style so geometry always wins; `compileStyle`
     * never emits a positioning property, so in practice they do not collide.
     */
    boxStyle?: StyleProp<ViewStyle>
    children?: ReactNode
}

/**
 * A `View` that renders one authored `Style`, backgrounds included.
 *
 * RN has no `background-image` property and no multiple-background support, so
 * gradients and images cannot be style props. They become absolutely-filled
 * siblings painted BENEATH the children, in the order `compileStyle` emits
 * them: gradient, then image, then children. That ordering reproduces by hand
 * the stacking a single CSS `background` shorthand would have given.
 *
 * The layers are deliberately NOT marked `pointerEvents="none"`. They are
 * earlier siblings, so children already sit above them in both RN's and the
 * DOM's hit-testing order, and a plain `View` with no responder does not
 * swallow a touch.
 */
export function StyledBox({style, boxStyle, children}: StyledBoxProps) {
    const {viewStyle, layers} = compileStyle(style)

    return (
        <View style={[viewStyle, boxStyle]}>
            {layers.map((layer, index) => {
                // Index keys are correct here and nowhere else in this folder:
                // the array is a compiled, fixed-order pair, never a list the
                // author can reorder.
                const key = `layer-${index}`

                if (layer.kind === "gradient") {
                    const colors = gradientColors(layer.colors)
                    if (colors === undefined) return null

                    return (
                        <LinearGradient
                            key={key}
                            colors={colors}
                            locations={gradientLocations(layer.locations, colors.length)}
                            start={layer.start}
                            end={layer.end}
                            style={StyleSheet.absoluteFill}
                        />
                    )
                }

                return (
                    <Image
                        key={key}
                        source={{uri: layer.uri}}
                        contentFit={contentFitFor(layer.resizeMode)}
                        style={StyleSheet.absoluteFill}
                    />
                )
            })}
            {children}
        </View>
    )
}
