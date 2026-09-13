import {Pressable, StyleSheet, Text, View} from "react-native"

import {setAbsolutePlacement, setPlacement} from "@/layout/edit/ops"
import {toPercentBox} from "@/layout/grid/coords"
import {allowsOverlap} from "@/layout/schema/placement"

import type {EditContext, LayoutSocket} from "@/layout/edit/ops"
import type {GridSize} from "@/layout/grid/coords"
import type {Placement} from "@/layout/schema/placement"

export interface PlacementPanelProps {
    ws: LayoutSocket
    ctx: EditContext
    widgetId: string
    placement: Placement
    grid: GridSize
}

/**
 * The three decisions that used to be one switch.
 *
 * Positioning mode, overlap permission and z-order are independent: a widget
 * can be grid-positioned AND allowed to sit on its neighbour, or viewport-
 * relative AND kept in the background. They live here rather than in
 * `ConfigPanel` because they describe the widget's PLACEMENT, not its config —
 * every widget has them, whatever its type, and no widget declares them as
 * field descriptors.
 */
export function PlacementPanel({ws, ctx, widgetId, placement, grid}: PlacementPanelProps) {
    const isGrid = placement.kind === "grid"
    const overlap = allowsOverlap(placement)

    /**
     * Switching mode has to CONVERT the geometry, not just relabel it.
     *
     * Cells and percentages describe the same box in different units, so the
     * widget must not move when the author flips the switch. Going to absolute
     * reuses `toPercentBox`, the same function the renderer uses to draw a grid
     * widget — so the box it lands on is exactly the box it already occupied.
     */
    function setMode(nextKind: Placement["kind"]): void {
        if (nextKind === placement.kind) return

        if (nextKind === "absolute") {
            if (placement.kind !== "grid") return
            const box = toPercentBox(placement, grid)
            setAbsolutePlacement(ws, ctx, widgetId, {
                kind: "absolute",
                left: box.left as `${number}%`,
                top: box.top as `${number}%`,
                width: box.width as `${number}%`,
                height: box.height as `${number}%`,
                ...(placement.z === undefined ? {} : {z: placement.z}),
                ...(placement.overlap === undefined ? {} : {overlap: placement.overlap}),
            })

            return
        }

        // Percentages back to cells: round to the nearest whole cell, floored
        // at one, so the widget keeps a usable footprint rather than collapsing.
        if (placement.kind !== "absolute") return
        const pct = (v: string) => Number.parseFloat(v) / 100
        const col = Math.round(pct(placement.left) * grid.cols)
        const row = Math.round(pct(placement.top) * grid.rows)
        const w = Math.max(1, Math.round(pct(placement.width) * grid.cols))
        const h = Math.max(1, Math.round(pct(placement.height) * grid.rows))

        // Sent through setPlacement, so the local collision check applies: a
        // widget returning to the grid has to fit unless it allows overlap.
        setPlacement(ws, ctx, widgetId, {col, row, w, h})
    }

    function setOverlap(next: boolean): void {
        // Absolute placement is exempt by definition, so the flag is only
        // meaningful on a grid widget.
        if (placement.kind !== "grid") return
        setPlacement(ws, ctx, widgetId, {...placement, overlap: next})
    }

    function nudgeZ(delta: number): void {
        const next = (placement.z ?? 0) + delta
        if (placement.kind === "absolute") {
            setAbsolutePlacement(ws, ctx, widgetId, {...placement, z: next})

            return
        }
        setPlacement(ws, ctx, widgetId, {...placement, z: next})
    }

    return (
        <View style={styles.panel}>
            <Text style={styles.heading}>Placement</Text>

            <Text style={styles.label}>Position</Text>
            <View style={styles.row}>
                <Choice label="Grid" on={isGrid} onPress={() => setMode("grid")}/>
                <Choice label="Viewport" on={!isGrid} onPress={() => setMode("absolute")}/>
            </View>

            <Text style={styles.label}>May overlap</Text>
            <View style={styles.row}>
                <Choice
                    label={overlap ? "Yes" : "No"}
                    on={overlap}
                    disabled={!isGrid}
                    onPress={() => setOverlap(!overlap)}
                />
            </View>
            {!isGrid && (
                <Text style={styles.note}>Viewport-positioned widgets always may.</Text>
            )}

            <Text style={styles.label}>{`Z-order: ${placement.z ?? "auto"}`}</Text>
            <View style={styles.row}>
                <Choice label="−" on={false} onPress={() => nudgeZ(-1)}/>
                <Choice label="+" on={false} onPress={() => nudgeZ(1)}/>
            </View>
            {placement.z === undefined && (
                <Text style={styles.note}>Auto: drawn in the order widgets are listed.</Text>
            )}
        </View>
    )
}

function Choice(
    {label, on, disabled, onPress}: {
        label: string
        on: boolean
        disabled?: boolean
        onPress: () => void
    },
) {
    return (
        <Pressable
            style={[styles.choice, on && styles.choiceOn, disabled === true && styles.choiceOff]}
            disabled={disabled}
            onPress={onPress}
        >
            <Text style={[styles.choiceText, on && styles.choiceTextOn]}>{label}</Text>
        </Pressable>
    )
}

const styles = StyleSheet.create({
    panel: {gap: 4},
    heading: {
        color: '#3f3f46',
        fontSize: 12,
        fontWeight: '600',
        textTransform: 'uppercase',
    },
    label: {color: '#111111', fontSize: 12, fontWeight: '600', marginTop: 4},
    note: {color: '#71717a', fontSize: 10},
    row: {flexDirection: 'row', gap: 4, flexWrap: 'wrap'},
    choice: {
        minHeight: 44,
        minWidth: 44,
        justifyContent: 'center',
        alignItems: 'center',
        paddingHorizontal: 8,
        borderRadius: 4,
        borderWidth: 1,
        borderColor: '#2563eb',
    },
    choiceOn: {backgroundColor: '#2563eb'},
    choiceOff: {opacity: 0.4},
    choiceText: {color: '#2563eb', fontSize: 12, fontWeight: '600'},
    choiceTextOn: {color: '#ffffff'},
})
