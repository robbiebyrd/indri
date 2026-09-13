import {Pressable, StyleSheet, Text, View} from "react-native"

import {allowsOverlap} from "@/layout/schema/placement"

import type {Widget} from "@/layout/schema/widget"

export interface WidgetListProps {
    widgets: Record<string, Widget>
    selectedId?: string
    onSelect: (id: string) => void
}

/**
 * Every widget in the scene, selectable by name.
 *
 * THE FRAME IS NOT THE ONLY WAY IN. A widget can be one cell across, sized to
 * nothing by a bad config, or sitting underneath another — all of which make it
 * effectively untappable on the canvas. A list has none of those failure modes:
 * whatever a widget's geometry, its row is the same size as every other row.
 *
 * It also shows what the canvas cannot: which widgets exist at all. A widget
 * whose placement puts it off-screen is invisible on the board but present
 * here, which is the difference between "broken" and "findable".
 */
export function WidgetList({widgets, selectedId, onSelect}: WidgetListProps) {
    const entries = Object.entries(widgets)

    if (entries.length === 0) {
        return (
            <View style={styles.list}>
                <Text style={styles.heading}>Widgets</Text>
                <Text style={styles.empty}>None yet. Add one above.</Text>
            </View>
        )
    }

    return (
        <View style={styles.list}>
            <Text style={styles.heading}>Widgets</Text>
            {entries.map(([id, widget]) => {
                const selected = id === selectedId

                return (
                    <Pressable
                        key={id}
                        style={[styles.row, selected && styles.rowOn]}
                        onPress={() => onSelect(id)}
                    >
                        <Text
                            style={[styles.id, selected && styles.idOn]}
                            numberOfLines={1}
                        >
                            {id}
                        </Text>
                        <Text style={[styles.meta, selected && styles.metaOn]} numberOfLines={1}>
                            {describe(widget)}
                        </Text>
                    </Pressable>
                )
            })}
        </View>
    )
}

/** Type, plus the placement facts that explain where a widget went. */
function describe(widget: Widget): string {
    const viewport = widget.placement.kind === "absolute"
    const parts = [widget.type, viewport ? "viewport" : "grid"]

    // Only worth saying when it is true: an absolute widget always overlaps, so
    // repeating it on every row would be noise.
    if (!viewport && allowsOverlap(widget.placement)) parts.push("overlaps")
    if (widget.placement.z !== undefined) parts.push(`z${widget.placement.z}`)

    return parts.join(" · ")
}

const styles = StyleSheet.create({
    list: {gap: 2},
    heading: {
        color: '#3f3f46',
        fontSize: 12,
        fontWeight: '600',
        textTransform: 'uppercase',
    },
    empty: {color: '#71717a', fontSize: 10},
    row: {
        minHeight: 44,
        justifyContent: 'center',
        paddingHorizontal: 8,
        borderRadius: 4,
        backgroundColor: '#f4f4f5',
    },
    rowOn: {backgroundColor: '#2563eb'},
    id: {color: '#111111', fontSize: 12, fontWeight: '600'},
    idOn: {color: '#ffffff'},
    meta: {color: '#71717a', fontSize: 10},
    metaOn: {color: '#dbeafe'},
})
