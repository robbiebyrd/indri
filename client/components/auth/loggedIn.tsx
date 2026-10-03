import {Text} from 'react-native'
import {findSlot} from "@/services/players";
import {getTeam} from "@/services/teams";
import {useGameState} from "@/providers/game-state/use-game-state";
import {useUserState} from "@/providers/user-state/use-user-state";

export default function LoggedIn() {
    const {gameState, dispatch: gameDispatch} = useGameState()
    const {userState, dispatch: userDispatch} = useUserState()

    const slot = findSlot(gameState, userState?.id)

    return (
        <>
            <Text>
                Welcome {slot?.player.name || userState?.displayName || userState?.name || undefined}
            </Text>
            <Text>
                {JSON.stringify(getTeam(slot?.slotId, gameState))}
            </Text>
        </>
    )
}

