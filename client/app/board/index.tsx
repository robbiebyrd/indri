import React, {useState} from "react"
import {View, Pressable, Text, StyleSheet} from "react-native"
import {useGameState} from "@/providers/game-state/use-game-state"
import {BoardView} from "@/components/board/board-view"

export default function BoardScreen() {
    const {gameState} = useGameState()
    const [editMode, setEditMode] = useState(false)

    if (!gameState) {
        return <View />
    }

    return (
        <View style={styles.root}>
            <BoardView game={gameState} editMode={editMode} />
            <Pressable
                style={[styles.editToggle, editMode && styles.editToggleActive]}
                onPress={() => setEditMode(m => !m)}
                accessibilityLabel={editMode ? "Exit edit mode" : "Enter edit mode"}
            >
                <Text style={styles.editToggleLabel}>{editMode ? "Done" : "Edit"}</Text>
            </Pressable>
        </View>
    )
}

const styles = StyleSheet.create({
    root: {flex: 1},
    editToggle: {
        position: "absolute",
        top: 12,
        right: 12,
        paddingHorizontal: 14,
        paddingVertical: 8,
        borderRadius: 8,
        backgroundColor: "rgba(0,0,0,0.55)",
    },
    editToggleActive: {
        backgroundColor: "rgba(0,100,255,0.85)",
    },
    editToggleLabel: {
        color: "white",
        fontWeight: "600",
        fontSize: 14,
    },
})
