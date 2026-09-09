import {Text} from 'react-native'
import {useGameState} from "@/providers/game-state/use-game-state";

export default function GameCode() {
    const {gameState} = useGameState()

    return (
        <Text>
            {JSON.stringify(gameState?.code)}
        </Text>
    )
}
