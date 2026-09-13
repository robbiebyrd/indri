/**
 * The layout editor's direct-manipulation layer.
 *
 * An OVERLAY, not a wrapper. The live board underneath keeps rendering exactly
 * as a player sees it (`BoardView` -> `SceneView` -> `WidgetHost`), and this
 * draws a transparent frame per widget on top of it. That is deliberate: an
 * editor that re-rendered the widgets itself would be a second renderer to keep
 * in step with the first, and the two would drift. The host edits against the
 * real board, sees real deltas land while editing, and the editor owns nothing
 * but gestures.
 *
 * THE ONE RULE HERE: exactly one op per gesture, on `onEnd`. Every connected
 * client deep-clones and replays the whole game state per delta, so an op per
 * frame is a delta storm that would make a 60fps drag cost 60 full replays on
 * every device in the game. Gesture position therefore lives in Reanimated
 * shared values on the UI thread and only the committed pixel delta crosses to
 * JS, once, via `runOnJS`.
 *
 * NOTHING IS APPLIED OPTIMISTICALLY. `setPlacement` sends and returns; the
 * published delta is the only confirmation, and it reaches the editing host by
 * exactly the same path as every other player. A local `canPlace` failure is
 * the sole exception, and only because it means no message was sent at all.
 */

import {useCallback, useMemo, useState} from "react"
import {StyleSheet, Text, View} from "react-native"
import {Gesture, GestureDetector} from "react-native-gesture-handler"
import Animated, {
    runOnJS,
    useAnimatedStyle,
    useSharedValue,
    withSequence,
    withTiming,
} from "react-native-reanimated"

import {setPlacement} from "@/layout/edit/ops"
import {knownWidgetTypes} from "@/layout/registry/registry"
import {parseLayout} from "@/layout/schema/layout"
import {allowsOverlap} from "@/layout/schema/placement"
import {cellSize, lineIndexes, rectToPixels, showGridLines, snapMove, snapResize} from "./snap"

import type {LayoutChangeEvent} from "react-native"
import type {EditContext, LayoutSocket} from "@/layout/edit/ops"
import type {PlacedWidget} from "@/layout/grid/collision"
import type {GridRect, GridSize} from "@/layout/grid/coords"
import type {SceneLayout} from "@/layout/schema/layout"
import type {BoardSize, CellSize} from "./snap"

/** How long a rejected gesture takes to slide back to where it started. */
const SNAP_BACK_MS = 160

/** Minimum touch target, per the platform guidelines both iOS and Android publish. */
const HANDLE_SIZE = 44

const FRAME_COLOR = "#2563eb"
const REJECT_COLOR = "#dc2626"

export interface EditorOverlayProps {
    /** Where ops go. `MessageHandler` satisfies this. */
    ws: LayoutSocket
    /**
     * Raw `game.data.layout`, the same untrusted value `BoardView` is given.
     *
     * Parsed again here rather than threaded down from the board, so that the
     * editor never needs `BoardView` to grow an editor-shaped prop. It is one
     * extra parse per layout change, memoised on the raw object's identity.
     */
    layout: unknown
    /** `stage.currentScene`. Widgets outside the current scene are not on screen to edit. */
    sceneId?: string
    /** Addresses the game the op is written to. */
    gameCode: string
    /** Selection is owned by the route, because the dock needs it too. */
    selectedId?: string
    onSelect: (id: string) => void
}

/**
 * What both halves of the editor need from a raw layout.
 *
 * Shared so the frame layer and the dock cannot disagree about which widgets
 * exist or what the grid is. Parsing twice is cheap next to that risk.
 */
export function useEditorScene(layout: unknown, sceneId: string | undefined, gameCode: string) {
    const parsed = useMemo(
        () => parseLayout(layout, {knownWidgetTypes: knownWidgetTypes()}).layout,
        [layout],
    )

    const scene: SceneLayout | undefined = parsed !== undefined && sceneId !== undefined
        ? parsed.scenes[sceneId]
        : undefined

    // The collision set every frame validates against. Absolute widgets are
    // deliberately absent — see `layout/grid/collision.ts`.
    const siblings = useMemo(() => gridSiblings(scene), [scene])

    const ctx: EditContext | undefined = parsed === undefined || sceneId === undefined
        ? undefined
        : {gameCode, sceneId, grid: parsed.grid, siblings}

    return {parsed, scene, siblings, ctx}
}

/**
 * Editable frames over the current scene, plus the grid the author is snapping
 * to.
 *
 * ONLY GRID-PLACED WIDGETS GET A FRAME. An absolute widget is positioned in
 * percentages and is exempt from collision as both subject and obstacle, so
 * dragging one is a different operation with a different op payload
 * (`setAbsolutePlacement`) and different rules. Pretending it snapped to cells
 * would write a grid placement over an absolute one and silently change what
 * the author authored.
 */
export function EditorOverlay(
    {ws, layout, sceneId, gameCode, selectedId, onSelect}: EditorOverlayProps,
) {
    // The board's pixel size, which is the only thing that turns a gesture in
    // pixels into cells. Absent until the first `onLayout`.
    const [board, setBoard] = useState<BoardSize | undefined>(undefined)

    const {parsed, scene, siblings, ctx} = useEditorScene(layout, sceneId, gameCode)

    const onLayout = useCallback((e: LayoutChangeEvent) => {
        const {width, height} = e.nativeEvent.layout
        setBoard((prev) =>
            prev !== undefined && prev.width === width && prev.height === height
                ? prev
                : {width, height})
    }, [])

    if (parsed === undefined || scene === undefined || ctx === undefined) return null

    const grid = parsed.grid
    const cell = board === undefined ? undefined : cellSize(board, grid)

    // Frames and guides only. The palette and the config panel live in the
    // dock beside the board, not over it — see editor-dock.tsx.
    return (
        <View style={styles.overlay} onLayout={onLayout}>
            <GridGuides grid={grid}/>
            {cell !== undefined && siblings.map((sibling) => (
                <EditableFrame
                    key={sibling.id}
                    id={sibling.id}
                    selected={sibling.id === selectedId}
                    onSelect={onSelect}
                    rect={sibling.rect}
                    cell={cell}
                    grid={grid}
                    ctx={ctx}
                    ws={ws}
                />
            ))}
        </View>
    )
}

/** Every grid-placed widget in the scene, in the shape `canPlace` wants. */
function gridSiblings(scene: SceneLayout | undefined): PlacedWidget[] {
    if (scene === undefined) return []

    const placed: PlacedWidget[] = []
    for (const [id, widget] of Object.entries(scene.widgets)) {
        // Exempt as both subject and obstacle: a widget that allows overlap is
        // simply absent from the set, so it neither blocks nor is blocked.
        if (widget.placement.kind !== "grid" || allowsOverlap(widget.placement)) continue
        const {col, row, w, h} = widget.placement
        placed.push({id, rect: {col, row, w, h}})
    }

    return placed
}

interface EditableFrameProps {
    id: string
    /** Selecting is what the palette's removal and the config panel act on. */
    selected: boolean
    onSelect: (id: string) => void
    /** Where the SERVER says this widget is. The frame always returns to it. */
    rect: GridRect
    cell: CellSize
    grid: GridSize
    ctx: EditContext
    ws: LayoutSocket
}

/**
 * One widget's drag surface and resize handle.
 *
 * The two gestures are SIBLINGS, not nested. A resize handle inside the drag
 * surface would put two Pan gestures on the same touch and need an explicit
 * relation to arbitrate them; siblings mean the topmost view simply wins, which
 * is the behaviour without any arbitration at all.
 */
function EditableFrame({id, selected, onSelect, rect, cell, grid, ctx, ws}: EditableFrameProps) {
    // Gesture state lives here and ONLY here, on the UI thread. It is a
    // transient offset from the server's rect, never a second copy of it.
    const tx = useSharedValue(0)
    const ty = useSharedValue(0)
    const dw = useSharedValue(0)
    const dh = useSharedValue(0)
    const rejected = useSharedValue(0)

    const box = rectToPixels(rect, cell)

    function flashRejected() {
        rejected.value = withSequence(
            withTiming(1, {duration: 60}),
            withTiming(0, {duration: SNAP_BACK_MS}),
        )
    }

    /**
     * The ONLY thing that crosses from the UI thread to JS during a gesture,
     * and it runs once, at the end.
     *
     * On acceptance the offset is dropped instantly rather than animated: the
     * frame returns to the server's rect and jumps to the new one when the
     * delta lands. Animating the return would race that delta and read as a
     * wobble. On rejection nothing was sent, so the slide back IS the feedback.
     */
    function commitMove(dx: number, dy: number) {
        const next = snapMove(rect, dx, dy, cell, grid)

        if (setPlacement(ws, ctx, id, next)) {
            tx.value = 0
            ty.value = 0
            return
        }

        tx.value = withTiming(0, {duration: SNAP_BACK_MS})
        ty.value = withTiming(0, {duration: SNAP_BACK_MS})
        flashRejected()
    }

    function commitResize(dx: number, dy: number) {
        const next = snapResize(rect, dx, dy, cell, grid)

        if (setPlacement(ws, ctx, id, next)) {
            dw.value = 0
            dh.value = 0
            return
        }

        dw.value = withTiming(0, {duration: SNAP_BACK_MS})
        dh.value = withTiming(0, {duration: SNAP_BACK_MS})
        flashRejected()
    }

    // `Gesture.Pan()` + `GestureDetector`, not RNGH 3's hook API: that needs
    // React Native >= 0.82 and this app is on 0.79.
    const drag = Gesture.Pan()
        .onChange((e) => {
            tx.value += e.changeX
            ty.value += e.changeY
        })
        .onEnd(() => {
            runOnJS(commitMove)(tx.value, ty.value)
        })

    // Raced against the drag rather than nested: a Pan needs movement and a Tap
    // needs none, so the two never both activate, and selecting a widget does
    // not cost a drag its first frames.
    const select = Gesture.Tap().onEnd(() => {
        runOnJS(onSelect)(id)
    })

    const resize = Gesture.Pan()
        .onChange((e) => {
            dw.value += e.changeX
            dh.value += e.changeY
        })
        .onEnd(() => {
            runOnJS(commitResize)(dw.value, dh.value)
        })

    const frameStyle = useAnimatedStyle(() => ({
        left: box.left,
        top: box.top,
        // Floored at one cell so a shrink past zero cannot invert the box while
        // the gesture is still running; `snapResize` enforces the same floor on
        // the value that is actually sent.
        width: Math.max(cell.width, box.width + dw.value),
        height: Math.max(cell.height, box.height + dh.value),
        transform: [{translateX: tx.value}, {translateY: ty.value}],
        borderColor: rejected.value > 0 ? REJECT_COLOR : FRAME_COLOR,
        borderWidth: selected ? 2 : 1,
    }))

    return (
        <Animated.View style={[styles.frame, frameStyle]}>
            <Text style={styles.frameLabel} numberOfLines={1}>{id}</Text>
            <GestureDetector gesture={Gesture.Race(drag, select)}>
                <Animated.View style={styles.dragSurface}/>
            </GestureDetector>
            <GestureDetector gesture={resize}>
                <Animated.View style={styles.handle}>
                    <View style={styles.handleGrip}/>
                </Animated.View>
            </GestureDetector>
        </Animated.View>
    )
}

/**
 * What the author is snapping to.
 *
 * Suppressed past `MAX_GRID_LINE_CELLS`: a 4096x4096 grid needs 8190 line
 * Views, and at that density they would render as a solid block anyway. The
 * dimensions in text say the same thing for the cost of one label.
 */
function GridGuides({grid}: {grid: GridSize}) {
    if (!showGridLines(grid)) {
        return (
            <View pointerEvents="none" style={styles.dimensions}>
                <Text style={styles.dimensionsText}>{`${grid.cols} x ${grid.rows}`}</Text>
            </View>
        )
    }

    return (
        <View pointerEvents="none" style={StyleSheet.absoluteFill}>
            {lineIndexes(grid.cols).map((i) => (
                <View key={`c${i}`} style={[styles.columnLine, {left: `${(i / grid.cols) * 100}%`}]}/>
            ))}
            {lineIndexes(grid.rows).map((i) => (
                <View key={`r${i}`} style={[styles.rowLine, {top: `${(i / grid.rows) * 100}%`}]}/>
            ))}
        </View>
    )
}

const styles = StyleSheet.create({
    // Covers the board exactly, so frame pixels and board pixels are the same
    // coordinate space and `onLayout` measures the thing the grid divides.
    overlay: {
        ...StyleSheet.absoluteFillObject,
    },
    frame: {
        position: 'absolute',
        borderWidth: 2,
        borderStyle: 'dashed',
        backgroundColor: 'rgba(37, 99, 235, 0.08)',
    },
    frameLabel: {
        color: FRAME_COLOR,
        fontSize: 10,
        paddingHorizontal: 3,
    },
    // Fills the frame so the whole widget is draggable, and sits above the
    // label so the label never eats a touch.
    dragSurface: {
        ...StyleSheet.absoluteFillObject,
    },
    // Hangs off the bottom-right corner. 44pt square regardless of how small
    // the widget is — a one-cell widget on a 64-column grid is a few pixels
    // wide, and a handle that size would be unusable on touch.
    handle: {
        position: 'absolute',
        right: -HANDLE_SIZE / 2,
        bottom: -HANDLE_SIZE / 2,
        width: HANDLE_SIZE,
        height: HANDLE_SIZE,
        alignItems: 'center',
        justifyContent: 'center',
    },
    handleGrip: {
        width: 14,
        height: 14,
        borderWidth: 2,
        borderColor: FRAME_COLOR,
        backgroundColor: '#ffffff',
    },
    columnLine: {
        position: 'absolute',
        top: 0,
        bottom: 0,
        width: StyleSheet.hairlineWidth,
        backgroundColor: 'rgba(37, 99, 235, 0.25)',
    },
    rowLine: {
        position: 'absolute',
        left: 0,
        right: 0,
        height: StyleSheet.hairlineWidth,
        backgroundColor: 'rgba(37, 99, 235, 0.25)',
    },
    dimensions: {
        position: 'absolute',
        top: 4,
        right: 6,
    },
    dimensionsText: {
        color: FRAME_COLOR,
        fontSize: 11,
    },
})
