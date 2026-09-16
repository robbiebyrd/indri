import React from "react"
import {StyledBox} from "./styled-box"
import {WidgetHost} from "./widget-host"
import type {Scene} from "@/layout/schema/layout"
import type {GridSize} from "@/layout/grid/coords"
import type {Style} from "@/layout/schema/style"
import type {Placement} from "@/layout/schema/placement"
import type {OverrideMap} from "@/layout/lua/overrides"

// Widget is inferred as `unknown` from the recursive zod schema — use this
// minimal structural type for the fields the renderer actually accesses.
interface WidgetShape {
    type: string
    placement: Placement
    style?: Style
    config?: Record<string, unknown>
}

interface Props {
    scene: Scene
    grid: GridSize
    overrides?: OverrideMap
    onWidgetPress?: (id: string) => void
}

export function SceneView({scene, grid, overrides, onWidgetPress}: Props) {
    return (
        <StyledBox style={scene.style} viewStyle={{flex: 1, position: "relative"}}>
            {Object.entries(scene.widgets).map(([id, widget]) => (
                <WidgetHost
                    key={id}
                    id={id}
                    widget={widget as WidgetShape}
                    grid={grid}
                    configOverride={overrides?.widgets[id]?.config}
                    styleOverride={overrides?.widgets[id]?.style}
                    onPress={onWidgetPress}
                />
            ))}
        </StyledBox>
    )
}
