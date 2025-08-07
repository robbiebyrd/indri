import {StyleSheet, Text, View} from 'react-native'
import {useMemo} from "react"
import {MessageHandler} from "@/services/message-handler";
import Login from "@/components/auth/login";
import {useGameState} from "@/providers/game-state/use-game-state";
import {useUserState} from "@/providers/user-state/use-user-state";
import GameCode from "@/components/display/labels/game-code";
import GameRefreshButton from "@/components/game/refresh";
import Join from "@/components/join/join";

export default function Index() {
    const {gameState, dispatch: gameDispatch} = useGameState()
    const {userState, dispatch: userDispatch} = useUserState()

    const ws: MessageHandler = useMemo(() => {
        return new MessageHandler(process.env.EXPO_PUBLIC_API_URL || "", userDispatch, gameDispatch)
    }, [])

    return (
        <View style={styles.container}>
            {!userState && <Login ws={ws}/>}
            {userState && "id" in userState && !gameState && <Join ws={ws}/>}
            {gameState && (
                <>
                    <GameCode/>
                    <Text>{JSON.stringify(gameState)}</Text>
                    <GameRefreshButton ws={ws}/>
                </>
            )}
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        alignItems: 'center',
        justifyContent: 'center',
    },
})
