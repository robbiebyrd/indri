import {Game, UpdateMessage, User} from "@/models/models";
import {GameStateParser} from "@/services/game-state-parser";
import {GameDispatchMessage} from "@/providers/game-state/game-state-actions";
import {UserDispatchMessage} from "@/providers/user-state/user-state-actions";
import {Dispatch} from "react";
import {GameListDispatchMessage} from "@/providers/game-list/game-list-actions";
import {GameInfo} from "@/providers/game-list/game-list-context";
import {parseJsonSafely} from "@/services/json";
import {JsonObject} from "type-fest";

type actionHandler = {
    name: string
    action: string
    parser: (data: any) => void
    dataKey?: string
}

export class MessageHandler {
    private ws?: WebSocket = undefined
    private stateList: GameStateParser<Game> = new GameStateParser<Game>()
    private readonly setGameState: Dispatch<GameDispatchMessage>
    private readonly setPlayerState: Dispatch<UserDispatchMessage>
    private readonly setGameList: Dispatch<GameListDispatchMessage>
    private parsers: actionHandler[]

    constructor(
        url: string,
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

        this.ws = new WebSocket(url)

        this.ws.onmessage = (e: MessageEvent) => {
            this.routeIncomingMessage(e)
        }

        //TODO: Handle errors appropriately.
        this.ws.onerror = (e: Event) => {
            console.log(e)
        }

        //TODO: Handle reconnects
        this.ws.onclose = (e: CloseEvent) => {
            console.log(e.code, e.reason)
        }

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

    send(message: object) {
        const msgString = JSON.stringify(message)
        this.ws?.send(msgString)
    }

    update(parsedMessage?: any) {
        this.stateList.update(parsedMessage as UpdateMessage)
        this.updateGameState()
    }

    messageType(parsedMessage: any): string | undefined {
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
        this.setGameState({payload: this.stateList.current(), type: "setGame"} as GameDispatchMessage)
    }

    updatePlayerState(data: any) {
        console.log("setting player state")
        console.log(data.user, data.sessionId)
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


