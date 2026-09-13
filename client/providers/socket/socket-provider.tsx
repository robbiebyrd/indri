import React, {createContext, useEffect, useRef} from 'react'
import AsyncStorage from '@react-native-async-storage/async-storage'

import {MessageHandler} from "@/services/message-handler";
import {useGameList} from "@/providers/game-list/use-game-list";
import {useGameState} from "@/providers/game-state/use-game-state";
import {useUserState} from "@/providers/user-state/use-user-state";

import type {ReactNode} from 'react'

export const SocketContext = createContext<MessageHandler | undefined>(undefined);

interface SocketProviderProps {
    children: ReactNode;
}

/**
 * One websocket for the whole app.
 *
 * IT LIVES AT THE ROOT, NOT IN A SCREEN, and that is the point. The session —
 * who you are, which game you are in — is per CONNECTION on the server, so a
 * socket owned by a route dies with that route and takes the authenticated
 * session with it. It used to be owned by `app/index.tsx`, which meant
 * navigating to any other route silently logged the player out and left every
 * other screen unable to send anything.
 *
 * Must be mounted INSIDE the three state providers: the handler dispatches
 * into all of them.
 */
export const SocketProvider: React.FC<SocketProviderProps> = ({children}) => {
    const {dispatch: gameDispatch} = useGameState()
    const {dispatch: userDispatch} = useUserState()
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
    const ws: MessageHandler = wsRef.current

    // Restore the session the moment the socket is up.
    //
    // The server's session is per CONNECTION, so a full page load — a refresh,
    // or a typed route URL — arrives with a brand new socket and no identity,
    // and without this the player is silently dropped back to the login screen
    // with their game gone. `reconnect` re-binds the stored token and the
    // server follows with a full keyframe if that session was in a game, so
    // game state comes back through the normal path with no special casing.
    //
    // It has to wait for `onOpen`: `send` drops anything written before the
    // handshake finishes, so firing this on mount would restore nothing.
    useEffect(() => {
        const handler = wsRef.current
        if (handler === undefined) return

        let cancelled = false

        const unsubscribe = handler.onOpen(() => {
            AsyncStorage.getItem('sessionId')
                .then((token) => {
                    // No token is the normal first-visit case, not an error.
                    if (cancelled || token === null || token === '') return

                    handler.send({action: 'reconnect', sessionId: token})
                })
                .catch((err: unknown) => {
                    // A failed restore leaves the app unauthenticated, which is
                    // the login screen — a correct state, just not the wanted
                    // one. Swallowing it silently would hide a broken store.
                    console.warn('could not restore the stored session', err)
                })
        })

        return () => {
            cancelled = true
            unsubscribe()
        }
    }, [])

    useEffect(() => {
        // Captured here rather than read in the cleanup: the ref is stable, but
        // reading `.current` at teardown time is the pattern that goes stale.
        const handler = wsRef.current

        return () => {
            handler?.close()
            wsRef.current = undefined
        }
    }, [])

    return (
        <SocketContext.Provider value={ws}>
            {children}
        </SocketContext.Provider>
    );
};
