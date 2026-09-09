import type {ViewStyle} from "react-native"

import type {Point, ResizeMode, Style} from "../schema/style.ts"

/**
 * A background RN cannot express as a style prop. There is no
 * `background-image` property and no multiple-background support, so gradients
 * and images become absolutely-positioned siblings painted beneath the box's
 * children. The renderer draws this array in order, then the children.
 */
export type BackgroundLayer =
    | {kind: "gradient"; colors: string[]; locations?: number[]; start?: Point; end?: Point}
    | {kind: "image"; uri: string; resizeMode: ResizeMode}

export interface CompiledStyle {
    viewStyle: ViewStyle
    layers: BackgroundLayer[]
}

/** Matches CSS's own default, and is what an author means by "a background image". */
const DEFAULT_RESIZE_MODE: ResizeMode = "cover"

const PADDING_PROPS = {
    top: "paddingTop",
    right: "paddingRight",
    bottom: "paddingBottom",
    left: "paddingLeft",
} as const

const RADIUS_PROPS = {
    topLeft: "borderTopLeftRadius",
    topRight: "borderTopRightRadius",
    bottomRight: "borderBottomRightRadius",
    bottomLeft: "borderBottomLeftRadius",
} as const

/**
 * Fan a scalar-or-per-side value out onto one RN property per side.
 *
 * The shorthand form is never emitted, even for a scalar: a partial per-side
 * value has no shorthand spelling, and mixing the two would make a compiled
 * style ambiguous to merge or override later.
 */
function expandSides<K extends string>(
    props: Readonly<Record<K, string>>,
    value: number | Partial<Record<K, number>>,
): Record<string, number> {
    const out: Record<string, number> = {}
    for (const side of Object.keys(props) as K[]) {
        const n = typeof value === "number" ? value : value[side]
        if (n !== undefined) out[props[side]] = n
    }
    return out
}

/**
 * Split a validated style into what RN can apply directly and what it has to
 * paint as a layer.
 *
 * `boxShadow` is the only shadow emitted. iOS `shadow*` and Android
 * `elevation` are deliberately absent: they are per-platform, they do not
 * compose with each other, and RN has offered a single cross-platform
 * `boxShadow` since 0.76.
 *
 * An absent style is normal (most widgets have none), so this returns empty
 * results rather than treating it as an error.
 */
export function compileStyle(s?: Style): CompiledStyle {
    const viewStyle: ViewStyle = {}
    const layers: BackgroundLayer[] = []

    if (!s) return {viewStyle, layers}

    if (s.backgroundColor !== undefined) viewStyle.backgroundColor = s.backgroundColor
    if (s.opacity !== undefined) viewStyle.opacity = s.opacity
    if (s.overflow !== undefined) viewStyle.overflow = s.overflow
    if (s.boxShadow !== undefined) viewStyle.boxShadow = s.boxShadow

    if (s.border !== undefined) {
        viewStyle.borderWidth = s.border.width
        viewStyle.borderColor = s.border.color
        if (s.border.style !== undefined) viewStyle.borderStyle = s.border.style
        if (s.border.radius !== undefined) {
            Object.assign(viewStyle, expandSides(RADIUS_PROPS, s.border.radius))
        }
    }

    if (s.padding !== undefined) Object.assign(viewStyle, expandSides(PADDING_PROPS, s.padding))

    // Gradient first, image second. The renderer paints the array in order, so
    // an image covers the gradient it is layered over — the CSS stacking a
    // single `background` shorthand would give, reproduced by hand.
    if (s.backgroundGradient !== undefined) layers.push({kind: "gradient", ...s.backgroundGradient})
    if (s.backgroundImage !== undefined) {
        layers.push({
            kind: "image",
            uri: s.backgroundImage.uri,
            resizeMode: s.backgroundImage.resizeMode ?? DEFAULT_RESIZE_MODE,
        })
    }

    return {viewStyle, layers}
}
