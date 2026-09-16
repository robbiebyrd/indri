import type {ViewStyle} from "react-native"
import type {Style, ResizeModeType} from "../schema/style.ts"

export type GradientLayer = {
    kind: "gradient"
    colors: string[]
    locations?: number[]
    start?: {x: number; y: number}
    end?: {x: number; y: number}
}

export type ImageLayer = {
    kind: "image"
    uri: string
    resizeMode?: ResizeModeType
}

export type BackgroundLayer = GradientLayer | ImageLayer

/**
 * Compile a Style value into a React Native ViewStyle plus an ordered background
 * layers array. Gradient layers always precede image layers. Shadow output uses
 * only boxShadow — never the iOS-only shadow* props or Android elevation.
 */
export function compileStyle(s?: Style): {viewStyle: ViewStyle; layers: BackgroundLayer[]} {
    if (s === undefined) {
        return {viewStyle: {}, layers: []}
    }

    const viewStyle: ViewStyle = {}
    const layers: BackgroundLayer[] = []

    if (s.backgroundColor !== undefined) {
        viewStyle.backgroundColor = s.backgroundColor
    }

    if (s.opacity !== undefined) {
        viewStyle.opacity = s.opacity
    }

    if (s.overflow !== undefined) {
        viewStyle.overflow = s.overflow
    }

    if (s.boxShadow !== undefined) {
        viewStyle.boxShadow = s.boxShadow
    }

    if (s.padding !== undefined) {
        if (typeof s.padding === "number") {
            viewStyle.padding = s.padding
        } else {
            if (s.padding.top !== undefined) viewStyle.paddingTop = s.padding.top
            if (s.padding.right !== undefined) viewStyle.paddingRight = s.padding.right
            if (s.padding.bottom !== undefined) viewStyle.paddingBottom = s.padding.bottom
            if (s.padding.left !== undefined) viewStyle.paddingLeft = s.padding.left
        }
    }

    if (s.border !== undefined) {
        viewStyle.borderWidth = s.border.width
        viewStyle.borderColor = s.border.color
        if (s.border.style !== undefined) {
            viewStyle.borderStyle = s.border.style
        }
        if (s.border.radius !== undefined) {
            if (typeof s.border.radius === "number") {
                viewStyle.borderRadius = s.border.radius
            } else {
                if (s.border.radius.topLeft !== undefined) viewStyle.borderTopLeftRadius = s.border.radius.topLeft
                if (s.border.radius.topRight !== undefined) viewStyle.borderTopRightRadius = s.border.radius.topRight
                if (s.border.radius.bottomLeft !== undefined) viewStyle.borderBottomLeftRadius = s.border.radius.bottomLeft
                if (s.border.radius.bottomRight !== undefined) viewStyle.borderBottomRightRadius = s.border.radius.bottomRight
            }
        }
    }

    // Layer ordering: gradient → image → (children are handled by the renderer)
    if (s.backgroundGradient !== undefined) {
        const gradient: GradientLayer = {
            kind: "gradient",
            colors: s.backgroundGradient.colors,
        }
        if (s.backgroundGradient.locations !== undefined) gradient.locations = s.backgroundGradient.locations
        if (s.backgroundGradient.start !== undefined) gradient.start = s.backgroundGradient.start
        if (s.backgroundGradient.end !== undefined) gradient.end = s.backgroundGradient.end
        layers.push(gradient)
    }

    if (s.backgroundImage !== undefined) {
        const image: ImageLayer = {
            kind: "image",
            uri: s.backgroundImage.uri,
        }
        if (s.backgroundImage.resizeMode !== undefined) image.resizeMode = s.backgroundImage.resizeMode
        layers.push(image)
    }

    return {viewStyle, layers}
}
