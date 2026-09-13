import {ScrollView, StyleSheet, Text, View} from "react-native"

import {getWidget} from "@/layout/registry/registry"

import {ConfigPanel} from "./config-panel"
import {useEditorScene} from "./drag-resize"
import {Palette} from "./palette"
import {PlacementPanel} from "./placement-panel"
import {WidgetList} from "./widget-list"

import type {LayoutSocket} from "@/layout/edit/ops"

/** The dock's share of the screen. The board gets the rest. */
export const DOCK_WIDTH_FRACTION = 0.15

export interface EditorDockProps {
    ws: LayoutSocket
    /** Raw `game.data.layout`, the same untrusted value the board is given. */
    layout: unknown
    sceneId?: string
    gameCode: string
    selectedId?: string
    onSelect: (id: string | undefined) => void
}

/**
 * The widget palette and the selected widget's config, docked to the right.
 *
 * IT IS A SIBLING OF THE BOARD, NOT AN OVERLAY ON IT. An opaque pane floating
 * over the canvas would cover widgets that are still editable underneath, and
 * the frame layer's percentage boxes are measured against the board, so a
 * widget hidden behind the dock would still accept a drag the author cannot
 * see. Taking the width out of the board instead keeps every widget reachable
 * and keeps the frames aligned with what is drawn.
 *
 * Scrolls on its own: a widget with many fields must not push the palette off
 * the screen, and at 15% of a phone this fills up quickly.
 */
export function EditorDock({ws, layout, sceneId, gameCode, selectedId, onSelect}: EditorDockProps) {
    const {scene, ctx} = useEditorScene(layout, sceneId, gameCode)

    if (ctx === undefined || scene === undefined) {
        return (
            <View style={styles.dock}>
                <Text style={styles.empty}>No scene to edit.</Text>
            </View>
        )
    }

    // EVERY id at this level, absolutes included. `ctx.siblings` omits them by
    // design, and generating a new id from that list alone would collide with
    // an absolutely placed widget, which the server rejects rather than
    // replaces.
    const widgetIds = Object.keys(scene.widgets)

    const selected = selectedId === undefined ? undefined : scene.widgets[selectedId]
    const selectedDef = selected === undefined ? undefined : getWidget(selected.type)

    return (
        <ScrollView style={styles.dock} contentContainerStyle={styles.content}>
            <Palette
                ws={ws}
                ctx={ctx}
                widgetIds={widgetIds}
                selectedId={selectedId}
                onAdd={onSelect}
                onRemove={() => onSelect(undefined)}
            />
            <WidgetList widgets={scene.widgets} selectedId={selectedId} onSelect={onSelect}/>
            {selected !== undefined && selectedId !== undefined && (
                <PlacementPanel
                    ws={ws}
                    ctx={ctx}
                    widgetId={selectedId}
                    placement={selected.placement}
                    grid={ctx.grid}
                />
            )}
            {selected !== undefined && selectedId !== undefined && (
                selectedDef === undefined
                    ? <Text style={styles.empty}>{`No editor for "${selected.type}".`}</Text>
                    : <ConfigPanel
                        def={selectedDef}
                        widgetId={selectedId}
                        config={selected.config ?? {}}
                        socket={ws}
                        ctx={ctx}
                    />
            )}
        </ScrollView>
    )
}

const styles = StyleSheet.create({
    dock: {
        // A fraction rather than a fixed width: the board is measured in
        // percentages too, so both halves scale together on any screen.
        width: `${DOCK_WIDTH_FRACTION * 100}%`,
        borderLeftWidth: 1,
        borderLeftColor: '#e4e4e7',
        backgroundColor: '#ffffff',
    },
    content: {
        gap: 12,
        padding: 8,
    },
    empty: {
        color: '#71717a',
        fontSize: 12,
    },
})
