import {User} from "@/models/models"

export type Action = 'setUser'

export type UserDispatchMessage = {
    type: Action
    payload: User
}

export function dataHandler(state?: User, action?: UserDispatchMessage): User | undefined {
    switch (action?.type) {
        case 'setUser':
            return {...action.payload};
        default:
            return state;
    }
}
