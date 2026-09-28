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
    userId: string
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

// In normal (MessagePack) mode: integer array. In debug (JSON) mode: string.
export type PathSegment = number[] | string

export declare interface UpdateMessage {
    o: 1 | 2 | 3        // OpCode: 1=update, 2=insert, 3=delete
    t: Date | string    // Timestamp extension (binary) or RFC3339 string (debug)
    u?: [PathSegment, unknown][]   // [[path, value], ...]
    r?: PathSegment[]              // [path, ...]
}

export declare interface LayoutFrame {
    o: 4
    v: string
    data: Record<string, unknown>
}

export declare interface SlimKeyframe {
    sv: string
    game: Game
}
