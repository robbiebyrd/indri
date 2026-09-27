import {Game, UpdateMessage, User} from "@/models/models";
import {GameStateParser} from "@/services/game-state-parser";
import {GameDispatchMessage} from "@/providers/game-state/game-state-actions";
import {UserDispatchMessage} from "@/providers/user-state/user-state-actions";
import {Dispatch} from "react";
import {GameListDispatchMessage} from "@/providers/game-list/game-list-actions";
import {GameInfo} from "@/providers/game-list/game-list-context";
import {parseJsonSafely} from "@/services/json";
import {JsonObject} from "type-fest";
import {decode} from "@msgpack/msgpack";
import type {Payload, TransportClient} from "@indri/protocol-client";

type actionHandler = {
    name: string
    action: string
    parser: (data: any) => void
    dataKey?: string
}

export class MessageHandler {
    private readonly transport: TransportClient
    private stateList: GameStateParser<Game> = new GameStateParser<Game>()
    private layoutVersion?: string
    private readonly setGameState: Dispatch<GameDispatchMessage>
    private readonly setPlayerState: Dispatch<UserDispatchMessage>
    private readonly setGameList: Dispatch<GameListDispatchMessage>
    private parsers: actionHandler[]

    constructor(
        transport: TransportClient,
        setPlayerState: Dispatch<UserDispatchMessage>,
        setGameState: Dispatch<GameDispatchMessage>,
        setGameList: Dispatch<GameListDispatchMessage>,
        parsers: actionHandler[] = []
    ) {
        this.setPlayerState = setPlayerState
        this.setGameState = setGameState
        this.setGameList = setGameList

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
                name: "indri_layout",
                action: "layout",
                parser: (d) => this.handleLayout(d)
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
            {
                name: "indri_disconnect",
                action: "disconnect",
                parser: () => { console.warn("server disconnected") }
            },
            ...parsers
        ]

        this.transport = transport

        this.transport.onMessage((data) => {
            this.routeIncomingMessage({data})
        })

        //TODO: Handle errors appropriately.
        this.transport.onError((e) => {
            console.log(e)
        })

        //TODO: Handle reconnects
        this.transport.onClose((reason) => {
            console.log(reason)
        })

        this.transport.connect().catch((e) => {
            console.error("could not connect to the server", e)
        })

        return this
    }

    routeIncomingMessage(message: {data: Payload}) {
        // Binary frames are MessagePack; text frames are JSON (?debug=1).
        const parsed: unknown = typeof message.data === "string"
            ? parseJsonSafely<JsonObject>(message.data)
            : decode(message.data)

        const action = this.messageType(parsed)
        if (!action) {
            return
        }

        for (const parser of this.parsers.filter(p => p.action === action)) {
            if (parser) {
                // A present key is used even when null (the server sends
                // "games": null for an empty list); only an absent one falls
                // back to the whole message.
                const obj = parsed as Record<string, unknown> | null
                const data = (parser.dataKey && obj && typeof obj === "object" && parser.dataKey in obj) ? obj[parser.dataKey] : parsed
                parser.parser(data)
            }
        }
    }

    updateAvailableGames(games: any[] | null) {
        this.setGameList({
            payload: (games ?? []) as GameInfo[],
            type: "setAvailableGames"
        } as GameListDispatchMessage)
    }

    // A close from our side never fires onClose, so a teardown can't trigger
    // close handling (e.g. future reconnect) during unmount.
    close() {
        this.transport.close()
    }

    // Dropped, with a warning, while the connection isn't open.
    send(message: object) {
        this.transport.send(JSON.stringify(message))
    }

    update(parsedMessage?: any) {
        this.stateList.update(parsedMessage as UpdateMessage)
        this.updateGameState()
    }

    messageType(parsedMessage: any): string | undefined {
        if (typeof parsedMessage !== "object" || parsedMessage === null) return undefined
        // sv must be checked before authenticated: a slim keyframe could theoretically
        // carry both fields, and keyframe routing must take priority.
        if ("sv" in parsedMessage) return "keyframe"
        if ("o" in parsedMessage && parsedMessage.o === 4) return "layout"
        if ("o" in parsedMessage && (parsedMessage.o === 1 || parsedMessage.o === 2 || parsedMessage.o === 3)) return "update"
        if ("authenticated" in parsedMessage && parsedMessage.authenticated === true) return "authenticated"
        if ("op" in parsedMessage && parsedMessage.op === "inquiryResponse") return "inquiryResponse"
        if ("disconnected" in parsedMessage) return "disconnect"
        return undefined
    }

    updateGameState() {
        const game = this.stateList.current()
        if (!game) {
            // No keyframe applied yet; don't dispatch an empty game that would
            // render the in-game UI over the join/create screen.
            return
        }
        this.setGameState({payload: game, type: "setGame"} as GameDispatchMessage)
    }

    updatePlayerState(data: any) {
        this.setPlayerState({
            payload: data.user as User,
            type: "setUser",
            sessionId: data.sessionId
        } as UserDispatchMessage)
    }

    // A layout arrives before a connection's first keyframe and again whenever
    // the host edits it; either way it replaces the one on screen.
    handleLayout(msg: any) {
        if (typeof msg?.v !== "string" || !msg.data || msg.v === this.layoutVersion) return
        this.layoutVersion = msg.v
        this.stateList.setLayout(msg.data)
        this.updateGameState()
    }

    keyframe(wrapperData: any) {
        const g = wrapperData.game as Game
        // The keyframe carries no layout; the parser lays the current one over it.
        this.stateList.setSchema(g as Record<string, unknown>)
        this.stateList.set(g as JsonObject, new Date(g.updatedAt ?? new Date().toISOString()))
        this.updateGameState()
    }
}


