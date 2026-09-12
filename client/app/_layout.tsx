import {Stack} from 'expo-router';
import {StyleSheet} from 'react-native';
import {GestureHandlerRootView} from 'react-native-gesture-handler';
import {GameStateProvider} from "@/providers/game-state/game-state-provider";
import {UserStateProvider} from "@/providers/user-state/user-state-provider";
import {GameListProvider} from "@/providers/game-list/game-list-provider";
import {SocketProvider} from "@/providers/socket/socket-provider";

// NOTE: do not call SplashScreen.preventAutoHideAsync() here again unless you
// also call hideAsync(). It used to be called at module scope with no matching
// hide, which left the splash up forever — and since its backgroundColor is
// #ffffff, that presented as a blank white app on iOS. Web hid it anyway, so
// the bug only showed on device. Nothing in this app loads fonts or assets
// asynchronously, so there is nothing to hold the splash for.

/**
 * `GestureHandlerRootView` WRAPS EVERYTHING, on every platform including web.
 *
 * Without it react-native-gesture-handler has nowhere to attach, and every
 * gesture in the app silently does nothing — no error, no warning, just a drag
 * that never starts. It is the first thing to check when the layout editor
 * looks broken. `flex: 1` is not optional either: the root view collapses to
 * zero height without it and the app renders blank.
 *
 * `SocketProvider` is INSIDE the three state providers (it dispatches into all
 * of them) and OUTSIDE the `Stack` (the server's session is per connection, so
 * a socket owned by a route would log the player out on navigation).
 */
export default function RootLayout() {
    return (
        <GestureHandlerRootView style={styles.root}>
            <GameListProvider>
                <UserStateProvider>
                    <GameStateProvider>
                        <SocketProvider>
                            <Stack screenOptions={{headerShown: false}}/>
                        </SocketProvider>
                    </GameStateProvider>
                </UserStateProvider>
            </GameListProvider>
        </GestureHandlerRootView>
    )
}

const styles = StyleSheet.create({
    root: {
        flex: 1,
    },
})
