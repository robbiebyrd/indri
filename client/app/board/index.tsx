import {View} from "react-native"
import {useGameState} from "@/providers/game-state/use-game-state"
import {BoardView} from "@/components/board/board-view"

export default function BoardScreen() {
    const {gameState} = useGameState()
    if (!gameState) {
        return <View />
    }
    return <BoardView game={gameState} />
}
