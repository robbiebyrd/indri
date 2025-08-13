import React, {ReactNode, useReducer} from 'react'
import {dataHandler} from "@/providers/game-list/game-list-actions";
import {GameListContext, initialState} from "@/providers/game-list/game-list-context";

interface GameListProviderProps {
    children: ReactNode;
}

export const GameListProvider: React.FC<GameListProviderProps> = ({children}) => {
    const [gameList, dispatch] = useReducer(dataHandler, initialState);

    return (
        <GameListContext.Provider value={{gameList, dispatch}}>
            {children}
        </GameListContext.Provider>
    );
};
