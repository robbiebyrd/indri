import {Button, Pressable, StyleSheet, Text, View} from 'react-native'
import {useMemo, useState} from "react"
import {MessageHandler} from "@/services/message-handler";
import Login from "@/components/auth/login";
import {useGameState} from "@/providers/game-state/use-game-state";
import {useUserState} from "@/providers/user-state/use-user-state";
import GameCode from "@/components/display/labels/game-code";
import GameRefreshButton from "@/components/game/refresh";
import Join from "@/components/join/join";
import {useGameList} from "@/providers/game-list/use-game-list";
import GameCreate from "@/components/join/create";

export default function Index() {
    const [showJoin, setShowJoin] = useState(true)

    const {gameState, dispatch: gameDispatch} = useGameState()
    const {userState, dispatch: userDispatch} = useUserState()
    const {dispatch: gameListDispatch} = useGameList()


    const ws: MessageHandler = useMemo(() => {
        return new MessageHandler(process.env.EXPO_PUBLIC_API_URL || "", userDispatch, gameDispatch, gameListDispatch)
    }, [])

    const currentScene = gameState?.stage?.scenes && gameState?.stage.currentScene ? gameState.stage.scenes[gameState.stage.currentScene] : undefined

    return (
        <View style={styles.container}>
            {!userState && <Login ws={ws}/>}
            {userState && "id" in userState && !gameState && (
                <>
                    {showJoin ? <Join ws={ws}/> : <GameCreate ws={ws}/>}
                    <Button title={showJoin ? "Create" : "Join"} onPress={() => setShowJoin(!showJoin)}/>
                </>
            )}
            {gameState && (
                <>
                    <GameCode/>
                    <View>
                        {currentScene?.data?.board?.map((row: string[], rowNumber: number) => (
                            <View style={styles.gridContainer} key={rowNumber}>{
                                row.map((column, columnNumber) => {
                                    if (column == "") {
                                        return (
                                            <View style={styles.gridItem} key={[rowNumber + columnNumber].join("-")}>
                                                <Pressable id={"a"} key={"a"} style={{width: "100%", height: "100%"}}
                                                           onPress={() => ws.send({
                                                               "action": "move",
                                                               "move": `${rowNumber},${columnNumber}`
                                                           })}>
                                                    <Text style={styles.gridItemText}>&nbsp;</Text>
                                                </Pressable>
                                            </View>
                                        )
                                    } else {
                                        return (
                                            <View style={styles.gridItem} key={[rowNumber + columnNumber].join("-")}>
                                                <Pressable id={"a"} key={"a"} style={{width: "100%", height: "100%"}}>
                                                    <Text style={styles.gridItemText}>{column}</Text>
                                                </Pressable>
                                            </View>
                                        )
                                    }
                                })
                            }
                            </View>)
                        )}
                    </View>
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
        height: '100%',
        width: 1000,
    },
    gridContainer: {
        height: '100%',
        width: '100%',
        flexDirection: 'row',
        flexWrap: 'wrap',
        justifyContent: 'space-around',
        padding: 0,
        flex: 3
    },
    gridItem: {
        width: "33%",
        height: 100,
        aspectRatio: 1,
        justifyContent: 'center',
        alignItems: 'center',
        backgroundColor: 'blue',
        borderWidth: 2,
        borderColor: 'black',
    },
    gridItemText: {
        color: 'white',
        fontSize: 80,
        textAlign: 'center',
    }
});
