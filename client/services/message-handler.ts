import {Game, UpdateMessage, User} from "@/models/models";
import {GameStateParser} from "@/services/game-state-parser";
import {GameDispatchMessage} from "@/providers/game-state/game-state-actions";
import {UserDispatchMessage} from "@/providers/user-state/user-state-actions";
import {Dispatch} from "react";
import {GameListDispatchMessage} from "@/providers/game-list/game-list-actions";
import {GameInfo} from "@/providers/game-list/game-list-context";
import {parseJsonSafely} from "@/services/json";
import {JsonObject} from "type-fest";
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
        if (typeof message.data !== "string") {
            console.warn("ignoring a binary message: this client only decodes JSON")
            return
        }

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

    keyframe(gameData: any) {
        const g = gameData as Game
        this.stateList.set(g as JsonObject, new Date(g.updatedAt ?? new Date().toISOString()))
        this.updateGameState()
    }
}


