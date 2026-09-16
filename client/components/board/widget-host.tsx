import React from "react"
import {toPercentBox} from "@/layout/grid/coords"
import {StyledBox} from "./styled-box"
import type {Style} from "@/layout/schema/style"
import type {GridSize} from "@/layout/grid/coords"
import type {Placement} from "@/layout/schema/placement"

// Widget is inferred as `unknown` from the recursive zod schema — use this
// minimal structural type for the fields this component actually accesses.
interface WidgetShape {
    placement: Placement
    style?: Style
}

interface Props {
    id: string
    widget: WidgetShape
    grid: GridSize
}

function absoluteBox(p: {left: string; top: string; width: string; height: string; z?: number}) {
    return {
        position: "absolute" as const,
        left: p.left,
        top: p.top,
        width: p.width,
        height: p.height,
        zIndex: p.z ?? 0,
    }
}

export const WidgetHost = React.memo(function WidgetHost({id, widget, grid}: Props) {
    const placementStyle = widget.placement.kind === "grid"
        ? {position: "absolute" as const, ...toPercentBox(widget.placement, grid)}
        : absoluteBox(widget.placement)

    return (
        <StyledBox style={widget.style} viewStyle={placementStyle}>
            {/* Widget content rendered here by scene-view via children or a renderWidget fn */}
        </StyledBox>
    )
}, (prev, next) => prev.widget === next.widget && prev.id === next.id)
// Identity compare: every delta produces fresh object identity for the whole game,
// so we skip re-render only when the widget object reference is unchanged.
