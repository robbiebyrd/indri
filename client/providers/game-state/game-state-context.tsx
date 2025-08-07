import {createContext} from 'react'
import {Game} from "@/models/models"

interface GameStateContextType {
    gameState?: Game;
    dispatch: any;
}

export const GameStateContext = createContext<GameStateContextType | undefined>(undefined);

export const initialState: Game | undefined = undefined
