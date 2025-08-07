export declare interface Game {
    code?: string
    teams?: Record<string | number, Team>
    players?: Record<string | number, Player>
    stage?: Stage
    data?: Record<string | number, Record<string | number, any>>
    privateData?: Record<string | number, Record<string | number, any>>
    playerData?: Record<string | number, Record<string | number, any>>
    updatedAt?: string
    createdAt?: string
}

export declare interface Stage {
    currentScene: string
    sceneOrder: string[]
    scenes?: Record<string, Scene>
    data?: Record<string | number, any>
    privateData?: Record<string | number, any>
    playerData?: Record<string | number, any>
}

export declare interface Scene {
    data?: Record<string | number, any>
    privateData?: Record<string | number, any>
    playerData?: Record<string | number, any>
}

export declare interface Player {
    name: string
    score: number
    connected: boolean
    host: boolean
    controller: boolean
    data?: Record<string | number, any>
    privateData?: Record<string | number, any>
}

export declare interface Team {
    name: string
    playerIds: string[]
    data?: Record<string | number, Record<string | number, any>>
    privateData?: Record<string | number, Record<string | number, any>>
    playerData?: Record<string | number, Record<string | number, any>>
}

export declare interface User {
    id: string
    email: string
    name: string
    displayName: string
    score: number
}

export declare interface UpdateMessage {
    id: string
    ts: string
    op: string
    type: string
    updated: object
    removed: string[]
}
