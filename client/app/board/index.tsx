import {useState} from "react"
import {Pressable, StyleSheet, Text, View} from "react-native"

import {BoardView} from "@/components/board/board-view"
import {EditorOverlay} from "@/components/board/editor/drag-resize"
import {useGameState} from "@/providers/game-state/use-game-state"
import {useSocket} from "@/providers/socket/use-socket"
import {useUserState} from "@/providers/user-state/use-user-state"

import type {Game} from "@/models/models"

/**
 * The live board route, with the layout editor over it.
 *
 * The board itself stays read-only: it observes the reduced game state and
 * renders it, and every edit leaves through the `layout` action and comes back
 * as a delta. The editor is a mode on THIS screen rather than a separate
 * `/board/edit` route, so the host authors against real state and watches other
 * players' deltas land while editing.
 */
export default function Board() {
    const {gameState} = useGameState()
    const {userState} = useUserState()
    const ws = useSocket()

    const [editing, setEditing] = useState(false)

    const layout = gameState?.data?.layout
    const sceneId = gameState?.stage?.currentScene
    const code = gameState?.code

    // Editing is host-only, and host-ness comes from the GAME, not the layout:
    // `game.players[userId].host` is the same flag the server's `layout` handler
    // authorizes against, so the button appears exactly when the action would
    // be accepted. Deriving it from layout data is impossible — a layout says
    // nothing about who anyone is.
    //
    // This is a UX gate and nothing more. A non-host who calls the action
    // anyway is rejected server-side; hiding the button only spares them a
    // round trip. A game code is required too: an op has nowhere to go without
    // one.
    const canEdit = isHost(gameState, userState?.id) && code !== undefined

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
            {editing && canEdit && code !== undefined && (
                <EditorOverlay ws={ws} layout={layout} sceneId={sceneId} gameCode={code}/>
            )}
            {canEdit && (
                <Pressable
                    style={[styles.toggle, editing && styles.toggleOn]}
                    onPress={() => setEditing((on) => !on)}
                >
                    <Text style={[styles.toggleText, editing && styles.toggleTextOn]}>
                        {editing ? "Done" : "Edit layout"}
                    </Text>
                </Pressable>
            )}
        </View>
    )
}

/** `players` is keyed by user id; an absent player is simply not the host. */
function isHost(game: Game | undefined, userId: string | undefined): boolean {
    if (game?.players === undefined || userId === undefined) return false

    return game.players[userId]?.host === true
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
    // Sits above the editor overlay so the way out of edit mode is never
    // covered by a widget frame.
    toggle: {
        position: 'absolute',
        right: 12,
        bottom: 12,
        minHeight: 44,
        justifyContent: 'center',
        paddingHorizontal: 16,
        borderRadius: 22,
        borderWidth: 1,
        borderColor: '#2563eb',
        backgroundColor: '#ffffff',
    },
    toggleOn: {
        backgroundColor: '#2563eb',
    },
    toggleText: {
        color: '#2563eb',
        fontSize: 14,
        fontWeight: '600',
    },
    toggleTextOn: {
        color: '#ffffff',
    },
})
