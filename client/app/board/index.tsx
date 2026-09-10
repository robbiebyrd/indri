import {StyleSheet, Text, View} from "react-native"

import {BoardView} from "@/components/board/board-view"
import {useGameState} from "@/providers/game-state/use-game-state"

/**
 * The live board route.
 *
 * Read-only: it observes the reduced game state and renders it. Nothing here
 * opens a socket or dispatches — `app/index.tsx` owns the connection, and this
 * screen only draws whatever the keyframe-plus-delta pipeline has produced.
 */
export default function Board() {
    const {gameState} = useGameState()

    const layout = gameState?.data?.layout
    const sceneId = gameState?.stage?.currentScene

    // An absent layout is the normal state before a game is joined, so it gets
    // an explanation rather than an error — but never a blank screen, which is
    // indistinguishable from a crashed renderer.
    if (layout === undefined || layout === null) {
        return (
            <View style={[styles.screen, styles.empty]}>
                <Text style={styles.emptyTitle}>Nothing to render</Text>
                <Text style={styles.emptyBody}>
                    {gameState === undefined
                        ? "Join or create a game first. The board draws itself once game state arrives."
                        : "This game has no layout. Add a \"layout\" object to the game's data."}
                </Text>
            </View>
        )
    }

    return (
        <View style={styles.screen}>
            <BoardView layout={layout} sceneId={sceneId}/>
        </View>
    )
}

const styles = StyleSheet.create({
    screen: {
        flex: 1,
    },
    empty: {
        alignItems: 'center',
        justifyContent: 'center',
        gap: 8,
        padding: 24,
    },
    emptyTitle: {
        color: '#222',
        fontSize: 18,
        fontWeight: '600',
    },
    emptyBody: {
        color: '#888',
        fontSize: 14,
        textAlign: 'center',
    },
})
