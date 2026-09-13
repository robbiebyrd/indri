import {createContext, memo, useContext} from "react"
import {Pressable, StyleSheet, Text, View} from "react-native"

import {getWidget} from "@/layout/registry/registry"
import {SUBGRID_TYPE} from "@/layout/schema/widget"
import {placementBox, placementZIndex} from "./placement"
import {StyledBox} from "./styled-box"

import type {ReactNode} from "react"
import type {GridSize} from "@/layout/grid/coords"
import type {Widget} from "@/layout/schema/widget"

/** Told which widget was pressed. The caller decides what that means. */
export type WidgetPressHandler = (widgetId: string) => void

/**
 * How a press reaches the Lua host.
 *
 * A CONTEXT, NOT A PROP, because the path from the board to a widget runs
 * through `SceneView` and then through every sub-grid on the way down, and a
 * prop would have to be re-threaded by each of them — including by any widget
 * type a game adds later. The board is the only provider; see `BoardView`.
 */
const WidgetPressContext = createContext<WidgetPressHandler | undefined>(undefined)

export const WidgetPressProvider = WidgetPressContext.Provider

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
    const onPress = useContext(WidgetPressContext)

    return (
        <StyledBox
            style={widget.style}
            boxStyle={[
                styles.widget,
                placementBox(widget.placement, grid),
                zIndex === undefined ? null : {zIndex},
            ]}
        >
            <PressTarget id={id} type={widget.type} onPress={onPress}>
                {Component === undefined
                    ? <UnrenderableWidget id={id} type={widget.type}/>
                    : <Component id={id} widget={widget}/>}
            </PressTarget>
        </StyledBox>
    )
}

/**
 * Makes one widget pressable, or leaves it alone.
 *
 * A SUB-GRID IS NEVER A PRESS TARGET. It is a coordinate space whose children
 * are the real widgets, and wrapping it would nest one press surface inside
 * another: on web the DOM click reaches both, so a press on cell `c01` would
 * also report a press on the sub-grid that contains it, and the script would
 * see an id it never placed. A container that swallowed or duplicated its
 * children's presses is the bug this rule exists to prevent.
 *
 * THE BRANCH IS ON `type` ALONE, AND THAT IS LOAD-BEARING. `type` cannot change
 * for a mounted widget, so the element type at this position is fixed for the
 * life of the subtree. An absent handler is handled by calling nothing —
 * `onPress?.(id)` — rather than by returning a Fragment, because `onPress`
 * comes from a context that starts undefined: `useLuaBridge` builds the bridge
 * in an effect, so the very first render of every board has no handler and the
 * next one does. Branching on it would flip Fragment -> Pressable at the same
 * position, and React would unmount and remount EVERY widget subtree on the
 * board. Today's widgets are stateless and would not notice; the first one with
 * focus, an animation or an uncontrolled input would lose it on connect.
 */
function PressTarget(
    {id, type, onPress, children}: {
        id: string
        type: string
        onPress: WidgetPressHandler | undefined
        children: ReactNode
    },
) {
    if (type === SUBGRID_TYPE) return <>{children}</>

    return (
        <Pressable style={styles.press} onPress={() => onPress?.(id)}>
            {children}
        </Pressable>
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
    // Fills the widget box so the whole widget is the target, not just the
    // area its content happens to cover.
    press: {
        flex: 1,
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
