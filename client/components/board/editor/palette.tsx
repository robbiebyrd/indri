import React from "react"
import {View, Text, Pressable, StyleSheet, ScrollView} from "react-native"
import {getAllWidgets} from "@/layout/registry/registry"
import "@/layout/registry/widgets"
import {firstFree} from "@/layout/edit/place"
import type {Sender, EditContext} from "@/layout/edit/ops"
import {addWidget, removeWidget} from "@/layout/edit/ops"

let _nextId = 0
function newWidgetId(): string {
    return `w${Date.now().toString(36)}${(_nextId++).toString(36)}`
}

type Props = {
    ws: Sender
    ctx: EditContext
}

/**
 * A horizontal scrollable list of widget types. Tapping a type adds a new
 * widget to the first free grid position. The palette is generated from the
 * registry so newly registered widgets appear without palette changes.
 */
export function Palette({ws, ctx}: Props) {
    const defs = getAllWidgets()

    function handleAdd(type: string) {
        const def = defs.find(d => d.type === type)
        if (!def) return

        const defaultSize = {w: 2, h: 2}
        const placement = firstFree(defaultSize, ctx.siblings, ctx.grid)
            ?? {kind: "absolute" as const, left: "0%" as const, top: "0%" as const, width: "20%" as const, height: "20%" as const}

        const widgetId = newWidgetId()
        addWidget(ws, ctx, widgetId, {
            type,
            placement: "kind" in placement && placement.kind === "absolute"
                ? placement
                : {kind: "grid", ...placement},
            config: def.defaults,
        })
    }

    return (
        <View style={styles.container}>
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.scroll}>
                {defs.map(def => (
                    <Pressable
                        key={def.type}
                        style={styles.chip}
                        onPress={() => handleAdd(def.type)}
                        accessibilityLabel={`Add ${def.type} widget`}
                    >
                        <Text style={styles.chipLabel}>+ {def.type}</Text>
                    </Pressable>
                ))}
            </ScrollView>
        </View>
    )
}

type RemoveButtonProps = {
    ws: Sender
    ctx: EditContext
    widgetId: string
}

/**
 * A small remove button rendered inside a widget in edit mode.
 */
export function RemoveButton({ws, ctx, widgetId}: RemoveButtonProps) {
    return (
        <Pressable
            style={styles.removeBtn}
            onPress={() => removeWidget(ws, ctx, widgetId)}
            accessibilityLabel="Remove widget"
            hitSlop={8}
        >
            <Text style={styles.removeBtnLabel}>✕</Text>
        </Pressable>
    )
}

const styles = StyleSheet.create({
    container: {
        position: "absolute",
        bottom: 0,
        left: 0,
        right: 0,
        backgroundColor: "rgba(0,0,0,0.7)",
        paddingVertical: 8,
    },
    scroll: {
        paddingHorizontal: 12,
        gap: 8,
    },
    chip: {
        paddingHorizontal: 14,
        paddingVertical: 10,
        borderRadius: 20,
        backgroundColor: "rgba(255,255,255,0.15)",
        minWidth: 44,
        alignItems: "center",
    },
    chipLabel: {
        color: "white",
        fontSize: 14,
        fontWeight: "600",
    },
    removeBtn: {
        position: "absolute",
        top: -8,
        right: -8,
        width: 24,
        height: 24,
        borderRadius: 12,
        backgroundColor: "rgba(220,50,50,0.9)",
        alignItems: "center",
        justifyContent: "center",
    },
    removeBtnLabel: {
        color: "white",
        fontSize: 12,
        fontWeight: "700",
        lineHeight: 14,
    },
})
