import {memo} from "react"
import {StyleSheet, Text, View} from "react-native"

import {getWidget} from "@/layout/registry/registry"
import {placementBox, placementZIndex} from "./placement"
import {StyledBox} from "./styled-box"

import type {GridSize} from "@/layout/grid/coords"
import type {Widget} from "@/layout/schema/widget"

export interface WidgetHostProps {
    /** The widget's key in its parent's widget map. Also its React key. */
    id: string
    widget: Widget
    /** The coordinate space `widget.placement` is measured in. */
    grid: GridSize
}

/**
 * Positions one widget and hands it to its registered renderer.
 *
 * Memoised on WIDGET IDENTITY, which is the whole point of this component.
 * `GameStateParser` deep-clones the entire game on every websocket message, so
 * value comparison would be both expensive and useless; identity at least lets
 * an untouched subtree bail out when the parse above it was skipped. The grid
 * is compared by value because it is two numbers and a stale one would silently
 * mis-position every widget on the board.
 */
function WidgetHostView({id, widget, grid}: WidgetHostProps) {
    const Component = getWidget(widget.type)?.Component
    const zIndex = placementZIndex(widget.placement)

    return (
        <StyledBox
            style={widget.style}
            boxStyle={[
                styles.widget,
                placementBox(widget.placement, grid),
                zIndex === undefined ? null : {zIndex},
            ]}
        >
            {Component === undefined
                ? <UnrenderableWidget id={id} type={widget.type}/>
                : <Component id={id} widget={widget}/>}
        </StyledBox>
    )
}

export const WidgetHost = memo(WidgetHostView, sameWidget)

function sameWidget(prev: WidgetHostProps, next: WidgetHostProps): boolean {
    return prev.widget === next.widget
        && prev.id === next.id
        && prev.grid.cols === next.grid.cols
        && prev.grid.rows === next.grid.rows
}

/**
 * Shown when a widget type has no renderer — either it is not registered at
 * all, or its definition exists without a `Component`.
 *
 * VISIBLE on purpose. Rendering nothing would leave a hole on the board with
 * no way to tell a missing widget from an empty one, and the unregistered case
 * is exactly what an author hits after a typo in `type`. `parseLayout` reports
 * unknown types as an issue, but a registered-yet-undrawn type produces no
 * issue at all, so this placeholder is its only signal.
 */
function UnrenderableWidget({id, type}: {id: string; type: string}) {
    return (
        <View style={styles.unrenderable}>
            <Text style={styles.unrenderableText} numberOfLines={3}>
                {`no renderer for type "${type}" (${id})`}
            </Text>
        </View>
    )
}

const styles = StyleSheet.create({
    // Every widget is absolutely positioned with a percentage box. There is no
    // flex layout inside the grid — the grid is a coordinate space, not a
    // container, so a widget's position never depends on its siblings.
    widget: {
        position: 'absolute',
    },
    unrenderable: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
        padding: 4,
        borderWidth: 1,
        borderStyle: 'dashed',
        borderColor: '#b91c1c',
        backgroundColor: '#fee2e2',
    },
    unrenderableText: {
        color: '#7f1d1d',
        fontSize: 10,
        textAlign: 'center',
    },
})
