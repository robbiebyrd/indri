import {Text} from 'react-native'
import {Game, Player} from "@/models/models";
import {getTeam} from "@/services/teams";
import {useGameState} from "@/providers/game-state/use-game-state";
import {useUserState} from "@/providers/user-state/use-user-state";

export default function LoggedIn() {
    const {gameState, dispatch: gameDispatch} = useGameState()
    const {userState, dispatch: userDispatch} = useUserState()

    const getPlayer = (playerId?: string): Player | undefined => {
        if (!gameState?.players || !playerId) {
            return undefined
        }

        for (const pId of Object.keys(gameState.players)) {
            if (pId == playerId) {
                return gameState.players[pId as keyof Game["players"]]
            }
        }

        return undefined
    }

    return (
        <>
            <Text>
                Welcome {getPlayer(userState?.id)?.name || userState?.displayName || userState?.name || undefined}
            </Text>
            <Text>
                {JSON.stringify(getTeam(userState?.id, gameState))}
            </Text>
        </>
    )
}

