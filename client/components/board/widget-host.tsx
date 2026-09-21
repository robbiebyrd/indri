import React from "react"
import {Pressable, ViewStyle} from "react-native"
import {toPercentBox} from "@/layout/grid/coords"
import {StyledBox} from "./styled-box"
import {getWidget} from "@/layout/registry/registry"
import "@/layout/registry/widgets"
import type {Style} from "@/layout/schema/style"
import type {GridSize} from "@/layout/grid/coords"
import type {Placement} from "@/layout/schema/placement"
import type {WidgetProps} from "@/layout/registry/registry"

// Widget is inferred as `unknown` from the recursive zod schema — use this
// minimal structural type for the fields this component actually accesses.
interface WidgetShape {
    type: string
    placement: Placement
    style?: Style
    config?: Record<string, unknown>
}

interface Props {
    id: string
    widget: WidgetShape
    grid: GridSize
    configOverride?: Record<string, unknown>
    styleOverride?: Style
    onPress?: (id: string) => void
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

export const WidgetHost = React.memo(function WidgetHost({id, widget, grid, configOverride, styleOverride, onPress}: Props) {
    const placementStyle = widget.placement.kind === "grid"
        ? {position: "absolute" as const, ...toPercentBox(widget.placement, grid)}
        : absoluteBox(widget.placement)

    const def = getWidget(widget.type)
    const effectiveStyle = styleOverride ? {...widget.style, ...styleOverride} : widget.style
    const effectiveConfig = configOverride ? {...widget.config, ...configOverride} : widget.config

    const content = def && effectiveConfig !== undefined ? (
        React.createElement(
            def.Component as React.ComponentType<WidgetProps<Record<string, unknown>>>,
            {
                id,
                config: effectiveConfig,
                context: {
                    widgetId: id,
                    sceneId: "",
                    override: () => { /* overrides managed by LuaSession */ },
                },
            },
        )
    ) : null

    const box = (
        <StyledBox style={effectiveStyle} viewStyle={{...placementStyle, overflow: "hidden"}}>
            {content}
        </StyledBox>
    )

    if (onPress) {
        return (
            <Pressable
                style={placementStyle as ViewStyle}
                onPress={() => onPress(id)}
            >
                {content ? (
                    <StyledBox style={effectiveStyle} viewStyle={{flex: 1, overflow: "hidden"}}>
                        {content}
                    </StyledBox>
                ) : null}
            </Pressable>
        )
    }

    return box
}, (prev, next) =>
    prev.widget === next.widget &&
    prev.id === next.id &&
    prev.configOverride === next.configOverride &&
    prev.styleOverride === next.styleOverride &&
    prev.onPress === next.onPress,
)
