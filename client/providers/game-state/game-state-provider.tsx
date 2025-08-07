import React, {ReactNode, useReducer} from 'react'
import {dataHandler} from "@/providers/game-state/game-state-actions";
import {GameStateContext, initialState} from "@/providers/game-state/game-state-context";

interface GameStateProviderProps {
    children: ReactNode;
}

export const GameStateProvider: React.FC<GameStateProviderProps> = ({children}) => {
    const [gameState, dispatch] = useReducer(dataHandler, initialState);

    return (
        <GameStateContext.Provider value={{gameState, dispatch}}>
            {children}
        </GameStateContext.Provider>
    );
};
