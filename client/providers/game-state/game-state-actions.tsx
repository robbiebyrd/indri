import {Game} from "@/models/models"

export type Action = 'setGame'

export type GameDispatchMessage = {
    type: Action
    payload: Game
}

export function dataHandler(state?: Game, action?: GameDispatchMessage): Game | undefined {
    switch (action?.type) {
        case 'setGame':
            return {...action.payload};
        default:
            return state;
    }
}
