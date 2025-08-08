import {Game, UpdateMessage, User} from "@/models/models";
import {GameStateParser} from "@/services/game-state-parser";
import {GameDispatchMessage} from "@/providers/game-state/game-state-actions";
import {UserDispatchMessage} from "@/providers/user-state/user-state-actions";
import {Dispatch} from "react";

export class MessageHandler {
    private ws?: WebSocket = undefined
    private stateList: GameStateParser<Game> = new GameStateParser<Game>()
    private readonly setGameState: Dispatch<GameDispatchMessage>
    private readonly setPlayerState: Dispatch<UserDispatchMessage>

    constructor(url: string, setPlayerState: Dispatch<UserDispatchMessage>, setGameState: Dispatch<GameDispatchMessage>) {
        this.setPlayerState = setPlayerState
        this.setGameState = setGameState

        const ws = new WebSocket(url)
        ws.onmessage = (e: MessageEvent) => {
            console.log(e.data)
            const parsed = JSON.parse(e.data)

            switch (this.messageType(parsed)) {
                case "authenticated":
                    this.updatePlayerState(parsed["user"] as User)
                    break
                case "update":
                    this.update(parsed as UpdateMessage)
                    break
                case "keyframe":
                    this.keyframe(parsed as Game)
                    break
                default:
                    break
            }
        }

        //TODO: Handle errors appropriately.
        ws.onerror = (e: Event) => {
            console.log(e)
        }

        //TODO: Handle reconnects
        ws.onclose = (e: CloseEvent) => {
            console.log(e.code, e.reason)
        }

        this.ws = ws
    }

    send(message: string) {
        this.ws?.send(message)
    }

    update(parsedMessage?: any) {
        this.stateList.update(parsedMessage)
        this.updateGameState()
    }

    messageType(parsedMessage: any): string | undefined {
        if ("authenticated" in parsedMessage && parsedMessage["authenticated"] == true) {
            return "authenticated"
        } else if ("op" in parsedMessage && parsedMessage["op"] == "update") {
            return "update"
        } else if ("code" in parsedMessage && "id" in parsedMessage) {
            return "keyframe"
        }
        return undefined
    }

    updateGameState() {
        this.setGameState({payload: this.stateList.current(), type: "setGame"} as GameDispatchMessage)
    }

    updatePlayerState(user: User) {
        this.setPlayerState({payload: user, type: "setUser"} as UserDispatchMessage)
    }


    keyframe(gameData: Game) {
        this.stateList.set(gameData, new Date(gameData.updatedAt ?? new Date().toISOString()))
        this.updateGameState()
    }

}


