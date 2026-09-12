import {Button, StyleSheet, View} from 'react-native'
import {useCallback, useState} from "react"
import Login from "@/components/auth/login";
import {useGameState} from "@/providers/game-state/use-game-state";
import {useUserState} from "@/providers/user-state/use-user-state";
import GameCode from "@/components/display/labels/game-code";
import GameRefreshButton from "@/components/game/refresh";
import Join from "@/components/join/join";
import GameCreate from "@/components/join/create";
import {BoardView} from "@/components/board/board-view";
import {useLuaBridge, useOverrides} from "@/providers/lua/use-lua-bridge";
import {useSocket} from "@/providers/socket/use-socket";

export default function Index() {
    const [showJoin, setShowJoin] = useState(true)

    const {gameState} = useGameState()
    const {userState} = useUserState()

    // The socket is owned by the root layout, not by this screen. It used to be
    // created here, which meant navigating to any other route closed it and
    // threw away the server-side session with it.
    const ws = useSocket()

    // The scripts in `game.data.layout` run here. The bridge observes the same
    // reduced game state the reducer produced, so a script can never disagree
    // with what is on screen.
    const bridge = useLuaBridge(ws)
    const overrides = useOverrides(bridge)

    const sceneId = gameState?.stage?.currentScene

    // The other half of the loop: a press becomes a `widgetPress` event that
    // bubbles widget -> scene -> board through the scripts, and whatever they
    // decide to do about it leaves through `indri.send`. Nothing about the game
    // is decided here — this component does not know what a cell is.
    const onWidgetPress = useCallback(
        (widgetId: string) => {
            if (bridge === undefined || sceneId === undefined) return
            bridge.host.emit("widgetPress", {kind: "widget", sceneId, widgetId}, widgetId)
        },
        [bridge, sceneId],
    )

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
                    <View style={styles.board}>
                        <BoardView
                            layout={gameState.data?.layout}
                            sceneId={sceneId}
                            overrides={overrides}
                            onWidgetPress={onWidgetPress}
                        />
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
        width: "100%",
    },
    // `width: '100%'` because the container centres its children on the cross
    // axis, which would otherwise shrink the board to its content — and the
    // board has no content of its own, only absolutely-positioned widgets.
    board: {
        flex: 1,
        width: '100%',
    },
});
