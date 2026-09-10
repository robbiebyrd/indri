/**
 * Placement -> absolute box.
 *
 * The only part of the renderer that is pure enough to test without a React
 * renderer, which is why it is a `.ts` file sitting next to the `.tsx` ones
 * rather than inlined into `widget-host.tsx`.
 *
 * `react-native` is imported for its TYPE only. `import type` is erased before
 * execution, so this module still loads in bare Node for its test.
 */

import {toPercentBox} from "../../layout/grid/coords.ts"

import type {DimensionValue} from "react-native"
import type {GridSize} from "../../layout/grid/coords.ts"
import type {Placement} from "../../layout/schema/placement.ts"

/** The four style properties that position a widget inside its parent. */
export interface AbsoluteBox {
    left: DimensionValue
    top: DimensionValue
    width: DimensionValue
    height: DimensionValue
}

/** The percentage form RN's `DimensionValue` accepts. */
const PERCENT = /^-?\d+(\.\d+)?%$/

/**
 * Where a widget sits, as RN style values.
 *
 * Both branches produce percentages, never pixels: the same layout has to
 * survive any viewport on web and native without a measurement pass, and the
 * absolute branch is percentage-only by construction (see `schema/placement.ts`).
 *
 * Absolute values are passed straight through — they are already `"NN%"`
 * strings and re-deriving them would only invent a rounding difference.
 */
export function placementBox(placement: Placement, grid: GridSize): AbsoluteBox {
    if (placement.kind === "absolute") {
        return {
            left: placement.left,
            top: placement.top,
            width: placement.width,
            height: placement.height,
        }
    }

    const box = toPercentBox(placement, grid)

    return {
        left: asDimension(box.left),
        top: asDimension(box.top),
        width: asDimension(box.width),
        height: asDimension(box.height),
    }
}

/**
 * Explicit stacking order, which only absolute placement carries.
 *
 * Grid-placed widgets are not meant to overlap at all (`parseLayout` warns when
 * they do) and fall back to insertion order, so giving them a z-index would
 * invent a layering feature the schema does not have.
 */
export function placementZIndex(placement: Placement): number | undefined {
    return placement.kind === "absolute" ? placement.z : undefined
}

/**
 * Re-type a percentage string as a `DimensionValue`.
 *
 * `toPercentBox` is typed as plain `string` because it is RN-free, but RN only
 * accepts the template-literal percentage form. The regex keeps the assertion
 * honest: anything that is not a percentage collapses to `0` rather than being
 * handed to RN, which would silently ignore it and leave the widget stretched
 * across its parent with no clue why.
 */
function asDimension(value: string): DimensionValue {
    return PERCENT.test(value) ? (value as `${number}%`) : 0
}
