import React from "react"
import {StyledBox} from "./styled-box"
import {WidgetHost} from "./widget-host"
import type {Scene} from "@/layout/schema/layout"
import type {GridSize} from "@/layout/grid/coords"
import type {Style} from "@/layout/schema/style"
import type {Placement} from "@/layout/schema/placement"

// Widget is inferred as `unknown` from the recursive zod schema — use this
// minimal structural type for the fields the renderer actually accesses.
interface WidgetShape {
    placement: Placement
    style?: Style
}

interface Props {
    scene: Scene
    grid: GridSize
}

export function SceneView({scene, grid}: Props) {
    return (
        <StyledBox style={scene.style} viewStyle={{flex: 1, position: "relative"}}>
            {Object.entries(scene.widgets).map(([id, widget]) => (
                <WidgetHost key={id} id={id} widget={widget as WidgetShape} grid={grid} />
            ))}
        </StyledBox>
    )
}
