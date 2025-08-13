import {useContext} from 'react';
import {GameListContext} from "@/providers/game-list/game-list-context";

export const useGameList = () => {
    const context = useContext(GameListContext);
    if (context === undefined) {
        throw new Error('useGameState must be used within a GameStateProvider');
    }
    return context;
};
