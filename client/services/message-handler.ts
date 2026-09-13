// The value imports are RELATIVE, not `@/`-aliased, and the rest are
// `import type`. That is what lets this module load under bare
// `node --experimental-strip-types`, which resolves neither tsconfig paths nor
// the React/Expo modules the aliased provider files pull in. See
// `layout/lua/bridge.node-test.ts`, which drives the real handler.
import {GameStateParser} from "./game-state-parser.ts";
import {parseJsonSafely} from "./json.ts";
import {WebSocketTransport} from "./transport.ts";

import type {Game, UpdateMessage, User} from "@/models/models";
import type {GameDispatchMessage} from "@/providers/game-state/game-state-actions";
import type {UserDispatchMessage} from "@/providers/user-state/user-state-actions";
import type {Dispatch} from "react";
import type {GameListDispatchMessage} from "@/providers/game-list/game-list-actions";
import type {GameInfo} from "@/providers/game-list/game-list-context";
import type {JsonObject} from "type-fest";
import type {ClientTransport} from "./transport.ts";

type actionHandler = {
    name: string
    action: string
    parser: (data: any) => void
    dataKey?: string
}

/**
 * Watches the REDUCED game state, not the messages that produced it.
 *
 * `kind` distinguishes a resync from an increment; a consumer that keeps local
 * state derived from the game (the Lua override layer, for one) has to drop it
 * on a keyframe and keep it on a delta.
 */
export type GameStateObserver = (game: Game, kind: "keyframe" | "delta") => void

export class MessageHandler {
    private readonly transport: ClientTransport
    private stateList: GameStateParser<Game> = new GameStateParser<Game>()
    private readonly setGameState: Dispatch<GameDispatchMessage>
    private readonly setPlayerState: Dispatch<UserDispatchMessage>
    private readonly setGameList: Dispatch<GameListDispatchMessage>
    private parsers: actionHandler[]
    private readonly observers = new Set<GameStateObserver>()
    private readonly openObservers = new Set<() => void>()
    private opened = false

    constructor(
        url: string,
        setPlayerState: Dispatch<UserDispatchMessage>,
        setGameState: Dispatch<GameDispatchMessage>,
        setGameList: Dispatch<GameListDispatchMessage>,
        parsers: actionHandler[] = [],
        transport: ClientTransport = new WebSocketTransport()
    ) {
        this.setPlayerState = setPlayerState
        this.setGameState = setGameState
        this.setGameList = setGameList
        this.transport = transport

        this.parsers = [
            {
                name: "indri_authenticated",
                action: "authenticated",
                parser: (d) => this.updatePlayerState(d),
            },
            {
                name: "indri_inquiryResponse",
                action: "inquiryResponse",
                parser: (d) => this.updateAvailableGames(d),
                dataKey: "games"
            },
            {
                name: "indri_keyframe",
                action: "keyframe",
                parser: (d) => this.keyframe(d)
            },
            {
                name: "indri_update",
                action: "update",
                parser: (d) => this.update(d)
            },
            ...parsers
        ]

        this.transport.connect(url)

        this.transport.onOpen(() => {
            this.opened = true
            for (const observer of this.openObservers) {
                try {
                    observer()
                } catch (err) {
                    console.warn("a socket-open observer threw", err)
                }
            }
        })

        this.transport.onMessage((data: string) => {
            this.routeIncomingMessage({data} as MessageEvent)
        })

        return this
    }

    routeIncomingMessage(message: MessageEvent) {
        const parsed = parseJsonSafely<JsonObject>(message.data)

        const action = this.messageType(parsed)
        if (!action) {
            return
        }

        for (const parser of this.parsers.filter(p => p.action === action)) {
            if (parser) {
                const data = (parser.dataKey && parsed && parsed[parser.dataKey]) ? parsed[parser.dataKey] : parsed
                parser.parser(data)
            }
        }
    }

    updateAvailableGames(games: any[]) {
        this.setGameList({
            payload: games as GameInfo[],
            type: "setAvailableGames"
        } as GameListDispatchMessage)
    }

    close() {
        this.transport.close()
    }

    /**
     * Run `observer` once the socket is open, or immediately if it already is.
     *
     * `send` drops anything written before the handshake completes, so anything
     * that must be the FIRST thing on the wire — restoring a session, say — has
     * to wait for this rather than firing on mount.
     *
     * Returns an unsubscribe so a caller that unmounts first does not fire.
     */
    onOpen(observer: () => void): () => void {
        if (this.opened) {
            observer()

            return () => undefined
        }

        this.openObservers.add(observer)

        return () => {
            this.openObservers.delete(observer)
        }
    }

    send(message: object) {
        this.transport.send(message)
    }

    /**
     * Watch the reduced game state. Returns an unsubscribe function.
     *
     * This is the ONLY inbound seam for anything that is not a message parser,
     * and it deliberately hands over the reduced game rather than the message:
     * an observer that re-interpreted a raw delta could disagree with the
     * reducer, and then two versions of "the game" would exist at once.
     */
    observe(observer: GameStateObserver): () => void {
        this.observers.add(observer)
        return () => {
            this.observers.delete(observer)
        }
    }

    update(parsedMessage?: any) {
        this.stateList.update(parsedMessage as UpdateMessage)
        this.updateGameState("delta")
    }

    messageType(parsedMessage: any): string | undefined {
        if (typeof parsedMessage !== "object" || parsedMessage === null) {
            return undefined
        }
        if ("authenticated" in parsedMessage && parsedMessage["authenticated"] == true) {
            return "authenticated"
        } else if ("op" in parsedMessage && parsedMessage["op"] == "update") {
            return "update"
        } else if ("op" in parsedMessage && parsedMessage["op"] == "inquiryResponse") {
            return "inquiryResponse"
        } else if ("code" in parsedMessage && "id" in parsedMessage) {
            return "keyframe"
        } else if ("op" in parsedMessage) {
            return parsedMessage["op"]
        }
        return undefined
    }

    updateGameState(kind: "keyframe" | "delta") {
        const game = this.stateList.current()
        if (!game) {
            // No keyframe applied yet; don't dispatch an empty game that would
            // render the in-game UI over the join/create screen.
            return
        }
        this.setGameState({payload: game, type: "setGame"} as GameDispatchMessage)

        for (const observer of this.observers) {
            try {
                observer(game, kind)
            } catch (e) {
                // The reducer has already been told. A misbehaving observer
                // must not also cost the next observer its notification.
                console.warn("a game state observer threw:", e)
            }
        }
    }

    updatePlayerState(data: any) {
        this.setPlayerState({
            payload: data.user as User,
            type: "setUser",
            sessionId: data.sessionId
        } as UserDispatchMessage)
    }

    keyframe(gameData: any) {
        const g = gameData as Game
        this.stateList.set(g as JsonObject, new Date(g.updatedAt ?? new Date().toISOString()))
        this.updateGameState("keyframe")
    }
}


