import {GameInfo} from "@/providers/game-list/game-list-context";

export type Action = 'setAvailableGames' | 'setGameInfo'

export type GameListDispatchMessage = {
    type: Action
    payload: GameInfo[] | GameInfo
}

export function dataHandler(state?: GameInfo[], action?: GameListDispatchMessage): GameInfo[] | undefined {
    switch (action?.type) {
        case 'setAvailableGames':
            if (Array.isArray(action.payload)) {
                return action.payload;
            } else {
                return [action.payload]
            }
        case 'setGameInfo':
            if (Array.isArray(action.payload)) {
                return action.payload[0] ? action.payload.slice(0, 1) : []
            } else {
                return [action.payload]
            }
        default:
            return state;
    }
}
