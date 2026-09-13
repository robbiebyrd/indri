import React, {createContext, useEffect, useRef} from 'react'

import {MessageHandler} from "@/services/message-handler";
import {FailoverTransport} from "@/services/failover-transport";
import {SseRestTransport} from "@/services/sse-transport";
import {WebSocketTransport} from "@/services/transport";
import {WebRTCTransport} from "@/services/webrtc-transport";
import {currentToken, loadToken} from "@/services/session-token";
import {useGameList} from "@/providers/game-list/use-game-list";
import {useGameState} from "@/providers/game-state/use-game-state";
import {useUserState} from "@/providers/user-state/use-user-state";

import type {ReactNode} from 'react'

export const SocketContext = createContext<MessageHandler | undefined>(undefined);

interface SocketProviderProps {
    children: ReactNode;
}

// How long one channel gets to come up before the next is tried, how long to
// wait after a cycle in which none did, and how many pre-open sends to hold.
const FAILOVER = {attemptTimeoutMs: 5000, backoffMs: 1000, queueLimit: 32}

/**
 * One connection for the whole app — over whichever transport works.
 *
 * IT LIVES AT THE ROOT, NOT IN A SCREEN, and that is the point. The session —
 * who you are, which game you are in — is per CONNECTION on the server, so a
 * connection owned by a route dies with that route and takes the authenticated
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

    // Create the connection once (lazy ref, not useMemo — connecting is a
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

        // The candidate list is a FUNCTION, re-evaluated on every selection
        // cycle. SSE authenticates at its handshake and has no anonymous form
        // to bind a session to later, so it is not even a legal candidate
        // until a token exists — which a fixed array could not express.
        //
        // Fresh instances per cycle: a transport that has been closed cannot
        // be redialled.
        const transport = new FailoverTransport(() => [
            new WebRTCTransport(),
            new WebSocketTransport(),
            ...(currentToken() ? [new SseRestTransport(currentToken)] : []),
        ], FAILOVER)

        wsRef.current = new MessageHandler(
            apiUrl ?? "", userDispatch, gameDispatch, gameListDispatch, [], transport,
        )
    }
    const ws: MessageHandler = wsRef.current

    // Restore the session on EVERY connect, not just the first.
    //
    // The server's session is per CONNECTION. A full page load — a refresh, or
    // a typed route URL — arrives with no identity, and so does every channel
    // the supervisor fails over to. Without this the player is silently
    // dropped back to the login screen with their game gone.
    //
    // `reconnect` re-binds the stored token; `refresh` then asks for a
    // keyframe, because a new connection's delta stream starts mid-flight and
    // GameStateParser has nothing to replay those deltas onto. Over REST the
    // two are independent requests rather than an ordered pair, which is
    // harmless: each carries the bearer token and resolves its own session.
    useEffect(() => {
        const handler = wsRef.current
        if (handler === undefined) return

        let cancelled = false
        let unsubscribe: (() => void) | undefined

        // Subscribe only AFTER the token is in the synchronous cache. That is
        // what lets `restore` send without awaiting: the supervisor flushes
        // its queued sends the moment a channel opens, and an awaited storage
        // read would put `reconnect` behind them. onOpen runs an observer
        // immediately when already connected, so subscribing late costs
        // nothing.
        loadToken()
            .catch((err: unknown) => {
                // A failed restore leaves the app unauthenticated, which is
                // the login screen — a correct state, just not the wanted one.
                // Swallowing it silently would hide a broken store.
                console.warn('could not read the stored session', err)

                return null
            })
            .then(() => {
                if (cancelled) return

                unsubscribe = handler.onOpen(() => {
                    const token = currentToken()

                    // No token is the normal first-visit case, not an error.
                    if (token === null) return

                    handler.send({action: 'reconnect', sessionId: token})
                    handler.send({action: 'refresh'})
                })
            })

        return () => {
            cancelled = true
            unsubscribe?.()
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
