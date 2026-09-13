import {User} from "@/models/models"

import {rememberToken} from "@/services/session-token"


export type Action = 'setUser'

export type UserDispatchMessage = {
    type: Action
    payload: User
    sessionId: string
}


export function dataHandler(state?: User, action?: UserDispatchMessage): User | undefined {
    switch (action?.type) {
        case 'setUser':
            if (action?.sessionId) {
                // Through session-token, not AsyncStorage directly: the
                // supervisor has to be able to read this token synchronously
                // when it picks a channel, and a write that only reached
                // storage would be invisible until the next app start.
                rememberToken(action.sessionId).catch((err: unknown) => {
                    // The session still works for this connection; only its
                    // survival across a restart is lost. Worth a warning, not
                    // worth failing the login over.
                    console.warn('could not persist the session token', err)
                })
            }
            return {...action.payload};
        default:
            return state
    }
}
