import {useContext} from 'react';
import {GameStateContext} from "@/providers/game-state/game-state-context";

export const useGameState = () => {
    const context = useContext(GameStateContext);
    if (context === undefined) {
        throw new Error('useGameState must be used within a GameStateProvider');
    }
    return context;
};
