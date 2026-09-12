import {Pressable, StyleSheet, Text, View} from "react-native"

import {removeWidget} from "@/layout/edit/ops"
import {createWidget, NEW_WIDGET_SIZE} from "@/layout/edit/place"
import {listWidgets} from "@/layout/registry/registry"

import type {EditContext, LayoutSocket} from "@/layout/edit/ops"
import type {WidgetDefinition} from "@/layout/registry/registry"

export interface PaletteProps {
    ws: LayoutSocket
    ctx: EditContext
    /**
     * EVERY widget id at this level, including absolutely placed ones.
     * `ctx.siblings` omits absolutes by design, so it is not this list, and
     * generating an id from it alone would collide with an absolute widget.
     */
    widgetIds: string[]
    /** The selected widget, if any. Its absence is what hides removal. */
    selectedId?: string
    /** Told the id of the widget that was just created. */
    onAdd?: (widgetId: string) => void
    /** Told the id whose removal was just sent. */
    onRemove?: (widgetId: string) => void
}

/**
 * The widget palette: one row per registered widget type, plus removal of the
 * current selection.
 *
 * THE LIST IS THE REGISTRY, not a hand-written menu. A game registers a widget
 * type and it appears here with no edit to this file — which is the only way a
 * framework's palette can stay correct for games the framework has never seen.
 * Order is the registry's insertion order, so registering a new type does not
 * reshuffle the rows an author already knows.
 *
 * Nothing is applied optimistically. Both affordances only send an op; the
 * published delta is what makes the widget appear or disappear, exactly as it
 * is for every other player in the game.
 */
export function Palette({ws, ctx, widgetIds, selectedId, onAdd, onRemove}: PaletteProps) {
    const add = (def: WidgetDefinition) => onAdd?.(createWidget(ws, ctx, def, widgetIds))

    const remove = () => {
        if (selectedId === undefined) return

        removeWidget(ws, ctx, selectedId)
        onRemove?.(selectedId)
    }

    return (
        <View style={styles.palette}>
            <Text style={styles.heading}>Add widget</Text>
            {listWidgets().map((def) => (
                <Pressable key={def.type} style={styles.type} onPress={() => add(def)}>
                    <Text style={styles.typeText}>{def.type}</Text>
                    <Text style={styles.typeHint}>{`${NEW_WIDGET_SIZE.w}x${NEW_WIDGET_SIZE.h}`}</Text>
                </Pressable>
            ))}
            {/*
              * Removal is an affordance ON THE SELECTION, not a row in the type
              * list: there is no such thing as removing "a text widget", only
              * removing the one that is selected. With nothing selected there is
              * no target, so the control is absent rather than disabled.
              */}
            {selectedId !== undefined && (
                <Pressable style={styles.remove} onPress={remove}>
                    <Text style={styles.removeText}>{`Remove "${selectedId}"`}</Text>
                </Pressable>
            )}
        </View>
    )
}

const styles = StyleSheet.create({
    palette: {
        gap: 4,
        padding: 8,
        backgroundColor: '#f4f4f5',
        borderRadius: 4,
    },
    heading: {
        color: '#3f3f46',
        fontSize: 12,
        fontWeight: '600',
        textTransform: 'uppercase',
    },
    type: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingVertical: 8,
        paddingHorizontal: 12,
        borderWidth: 1,
        borderColor: '#d4d4d8',
        borderRadius: 4,
        backgroundColor: '#ffffff',
    },
    typeText: {
        color: '#18181b',
    },
    typeHint: {
        color: '#a1a1aa',
        fontSize: 11,
    },
    remove: {
        marginTop: 8,
        paddingVertical: 8,
        paddingHorizontal: 12,
        borderWidth: 1,
        borderColor: '#b91c1c',
        borderRadius: 4,
        backgroundColor: '#fee2e2',
    },
    removeText: {
        color: '#7f1d1d',
        textAlign: 'center',
    },
})
