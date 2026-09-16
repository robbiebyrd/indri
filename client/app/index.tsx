import {Button, StyleSheet, View} from 'react-native'
import {useEffect, useRef, useState} from "react"
import {MessageHandler} from "@/services/message-handler";
import Login from "@/components/auth/login";
import {useGameState} from "@/providers/game-state/use-game-state";
import {useUserState} from "@/providers/user-state/use-user-state";
import GameCode from "@/components/display/labels/game-code";
import GameRefreshButton from "@/components/game/refresh";
import Join from "@/components/join/join";
import {useGameList} from "@/providers/game-list/use-game-list";
import GameCreate from "@/components/join/create";
import {BoardView} from "@/components/board/board-view";

export default function Index() {
    const [showJoin, setShowJoin] = useState(true)

    const {gameState, dispatch: gameDispatch} = useGameState()
    const {userState, dispatch: userDispatch} = useUserState()
    const {dispatch: gameListDispatch} = useGameList()


    // Create the socket once (lazy ref, not useMemo — opening a socket is a
    // side effect) and close it on unmount to avoid leaking connections.
    const wsRef = useRef<MessageHandler | undefined>(undefined)
    if (!wsRef.current) {
        // An empty URL is not a harmless default: WebSocket("") resolves
        // against the page origin, so the app silently dials Metro on :8081
        // and looks like a broken server rather than missing config.
        const apiUrl = process.env.EXPO_PUBLIC_API_URL
        if (!apiUrl) {
            console.error(
                "EXPO_PUBLIC_API_URL is not set. Copy client/.env.example to client/.env " +
                "and restart Metro — EXPO_PUBLIC_* values are inlined at build time.",
            )
        }
        wsRef.current = new MessageHandler(apiUrl ?? "", userDispatch, gameDispatch, gameListDispatch)
    }
    const ws: MessageHandler = wsRef.current!

    useEffect(() => {
        return () => {
            ws.close()
            wsRef.current = undefined
        }
    }, [])

    return (
        <View style={styles.container}>
            {!userState && <Login ws={ws}/>}
            {userState && "id" in userState && !gameState && (
                <>
                    {showJoin ? <Join ws={ws}/> : <GameCreate ws={ws}/>}
                    <Button title={showJoin ? "Create" : "Join"} onPress={() => setShowJoin(!showJoin)}/>
                </>
            )}
            {gameState && (
                <>
                    <GameCode/>
                    <BoardView game={gameState} onSend={(msg) => ws.send(msg)}/>
                    <GameRefreshButton ws={ws}/>
                </>
            )}
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
        height: '100%',
        width: 1000,
    },
})
