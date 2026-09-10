/**
 * Compiled background layer -> the props the two layer components actually take.
 *
 * `compileStyle` produces a layer description validated by zod, but zod's
 * inferred types are looser than what `expo-linear-gradient` and `expo-image`
 * declare: the gradient wants a tuple of at least two colours, and `expo-image`
 * has a smaller resize vocabulary than CSS. Bridging that is real logic with
 * real failure modes, so it lives here where a bare-Node test can reach it.
 */

import type {ResizeMode} from "../../layout/schema/style.ts"

/** `expo-linear-gradient` requires at least two stops, expressed as a tuple. */
export type GradientColors = readonly [string, string, ...string[]]

/** Same shape rule as `GradientColors`, and must be the same length. */
export type GradientLocations = readonly [number, number, ...number[]]

/**
 * `expo-image`'s `contentFit` vocabulary. Declared structurally rather than
 * imported so this module stays free of runtime imports; the compiler still
 * checks it against `ImageContentFit` at the call site.
 */
export type ContentFit = "cover" | "contain" | "fill" | "none"

/**
 * Narrow a colour list to the tuple the gradient component requires.
 *
 * `StyleSchema` already demands two colours, so `undefined` here means the
 * style was built in code rather than parsed. Returning `undefined` makes the
 * caller skip the layer; passing a short array through would crash the native
 * view instead.
 */
export function gradientColors(colors: readonly string[]): GradientColors | undefined {
    if (colors.length < 2) return undefined

    return [colors[0], colors[1], ...colors.slice(2)]
}

/**
 * Narrow the colour-stop positions, dropping them unless they match the colours.
 *
 * A locations array of a different length than `colors` is undefined behaviour
 * in the native gradient — on some platforms it throws, on others it silently
 * renders the wrong stops. An evenly distributed gradient is a far better
 * failure than either, so a mismatch drops the positions rather than the layer.
 */
export function gradientLocations(
    locations: readonly number[] | undefined,
    colorCount: number,
): GradientLocations | undefined {
    if (locations === undefined || locations.length < 2 || locations.length !== colorCount) {
        return undefined
    }

    return [locations[0], locations[1], ...locations.slice(2)]
}

/**
 * Map the style vocabulary's resize mode onto `expo-image`'s.
 *
 * `repeat` has NO equivalent: `expo-image` cannot tile, and `contentFit` is
 * modelled on CSS `object-fit`, which has no tiling value either. It degrades
 * to `none` — the image at its natural size, centred — which is the same
 * scale a tile would have drawn at, just without the repetition.
 */
export function contentFitFor(mode: ResizeMode): ContentFit {
    switch (mode) {
        case "cover":
            return "cover"
        case "contain":
            return "contain"
        case "stretch":
            return "fill"
        case "center":
        case "repeat":
            return "none"
    }
}
