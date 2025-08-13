import {createContext} from 'react'

export interface GameInfo {
    code: string
    full: boolean
    teams: {
        name: string
        full: boolean
    }[]
}

interface GameListContextType {
    gameList?: GameInfo[]
    dispatch: any
}

export const GameListContext = createContext<GameListContextType | undefined>(undefined);

export const initialState: GameInfo[] | undefined = undefined
