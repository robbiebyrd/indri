import React, {useCallback} from "react"
import {StyleSheet, View, Text} from "react-native"
import Animated, {
    useSharedValue,
    useAnimatedStyle,
    runOnJS,
} from "react-native-reanimated"
import {Gesture, GestureDetector} from "react-native-gesture-handler"
import type {GridRect, GridSize} from "@/layout/grid/coords"
import type {Sender, EditContext} from "@/layout/edit/ops"
import {moveWidget} from "@/layout/edit/ops"

const HANDLE_SIZE = 44

export type ContainerSize = {width: number; height: number}

/**
 * Convert an accumulated pixel drag (tx, ty) to a new grid rect for the widget.
 * The widget's width and height are preserved; only col/row move.
 */
export function snapToCell(
    tx: number,
    ty: number,
    initial: GridRect,
    grid: GridSize,
    container: ContainerSize,
): GridRect {
    if (container.width === 0 || container.height === 0) return initial
    const cellW = container.width / grid.cols
    const cellH = container.height / grid.rows
    const col = Math.max(0, Math.min(
        initial.col + Math.round(tx / cellW),
        grid.cols - initial.w,
    ))
    const row = Math.max(0, Math.min(
        initial.row + Math.round(ty / cellH),
        grid.rows - initial.h,
    ))
    return {col, row, w: initial.w, h: initial.h}
}

/**
 * Convert an accumulated pixel resize drag (dw, dh) to a new grid rect.
 * The widget's col/row are preserved; only w/h change.
 */
export function snapToResize(
    dw: number,
    dh: number,
    initial: GridRect,
    grid: GridSize,
    container: ContainerSize,
): GridRect {
    if (container.width === 0 || container.height === 0) return initial
    const cellW = container.width / grid.cols
    const cellH = container.height / grid.rows
    const w = Math.max(1, Math.min(
        initial.w + Math.round(dw / cellW),
        grid.cols - initial.col,
    ))
    const h = Math.max(1, Math.min(
        initial.h + Math.round(dh / cellH),
        grid.rows - initial.row,
    ))
    return {...initial, w, h}
}

type DragResizeProps = {
    ws: Sender
    ctx: EditContext
    id: string
    initial: GridRect
    containerSize: ContainerSize
    children: React.ReactNode
}

/**
 * Wraps a widget in edit mode with drag (move) and resize gestures.
 * Only one op is emitted per gesture, never per frame.
 * Rejected moves snap back to the original position.
 */
export function DragResize({ws, ctx, id, initial, containerSize, children}: DragResizeProps) {
    const tx = useSharedValue(0)
    const ty = useSharedValue(0)
    const dw = useSharedValue(0)
    const dh = useSharedValue(0)

    const commitMove = useCallback(() => {
        const next = snapToCell(tx.value, ty.value, initial, ctx.grid, containerSize)
        if (!moveWidget(ws, ctx, id, next)) {
            tx.value = 0
            ty.value = 0
        }
    }, [ws, ctx, id, initial, containerSize, tx, ty])

    const commitResize = useCallback(() => {
        const next = snapToResize(dw.value, dh.value, initial, ctx.grid, containerSize)
        if (!moveWidget(ws, ctx, id, next)) {
            dw.value = 0
            dh.value = 0
        }
    }, [ws, ctx, id, initial, containerSize, dw, dh])

    // One op per gesture end, never per frame (shared values stay on UI thread).
    const drag = Gesture.Pan()
        .onChange(e => {
            tx.value += e.changeX
            ty.value += e.changeY
        })
        .onEnd(() => runOnJS(commitMove)())

    const resize = Gesture.Pan()
        .onChange(e => {
            dw.value += e.changeX
            dh.value += e.changeY
        })
        .onEnd(() => runOnJS(commitResize)())

    const moveStyle = useAnimatedStyle(() => ({
        transform: [{translateX: tx.value}, {translateY: ty.value}],
    }))

    const resizeHandleStyle = useAnimatedStyle(() => ({
        transform: [{translateX: dw.value}, {translateY: dh.value}],
    }))

    return (
        <GestureDetector gesture={drag}>
            <Animated.View style={[StyleSheet.absoluteFill, moveStyle]}>
                {children}
                <GestureDetector gesture={resize}>
                    <Animated.View style={[styles.resizeHandle, resizeHandleStyle]} />
                </GestureDetector>
            </Animated.View>
        </GestureDetector>
    )
}

/**
 * Draws a grid-line overlay over the board container in edit mode.
 * Suppressed when cols×rows > 4096 to avoid thousands of lines; shows
 * the grid dimensions as text instead.
 */
type GridOverlayProps = {
    grid: GridSize
    containerSize: ContainerSize
}

export function GridOverlay({grid, containerSize}: GridOverlayProps) {
    if (grid.cols * grid.rows > 4096 || containerSize.width === 0) {
        return (
            <View style={styles.dimLabel} pointerEvents="none">
                <Text style={styles.dimText}>{grid.cols} × {grid.rows}</Text>
            </View>
        )
    }

    const cellW = containerSize.width / grid.cols
    const cellH = containerSize.height / grid.rows

    const vLines = []
    for (let c = 1; c < grid.cols; c++) {
        vLines.push(
            <View key={c} style={[styles.vLine, {left: c * cellW}]} />
        )
    }
    const hLines = []
    for (let r = 1; r < grid.rows; r++) {
        hLines.push(
            <View key={r} style={[styles.hLine, {top: r * cellH}]} />
        )
    }

    return (
        <View style={StyleSheet.absoluteFill} pointerEvents="none">
            {vLines}
            {hLines}
        </View>
    )
}

const styles = StyleSheet.create({
    resizeHandle: {
        position: "absolute",
        bottom: 0,
        right: 0,
        width: HANDLE_SIZE,
        height: HANDLE_SIZE,
        backgroundColor: "rgba(0,100,255,0.4)",
        borderTopLeftRadius: 4,
    },
    dimLabel: {
        position: "absolute",
        top: 4,
        right: 4,
    },
    dimText: {
        fontSize: 12,
        color: "rgba(0,0,0,0.4)",
        fontVariant: ["tabular-nums"],
    },
    vLine: {
        position: "absolute",
        top: 0,
        bottom: 0,
        width: StyleSheet.hairlineWidth,
        backgroundColor: "rgba(0,0,0,0.12)",
    },
    hLine: {
        position: "absolute",
        left: 0,
        right: 0,
        height: StyleSheet.hairlineWidth,
        backgroundColor: "rgba(0,0,0,0.12)",
    },
})
