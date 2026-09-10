import {Stack} from 'expo-router';
import {GameStateProvider} from "@/providers/game-state/game-state-provider";
import {UserStateProvider} from "@/providers/user-state/user-state-provider";
import {GameListProvider} from "@/providers/game-list/game-list-provider";

// NOTE: do not call SplashScreen.preventAutoHideAsync() here again unless you
// also call hideAsync(). It used to be called at module scope with no matching
// hide, which left the splash up forever — and since its backgroundColor is
// #ffffff, that presented as a blank white app on iOS. Web hid it anyway, so
// the bug only showed on device. Nothing in this app loads fonts or assets
// asynchronously, so there is nothing to hold the splash for.

export default function RootLayout() {
    return (
        <GameListProvider>
            <UserStateProvider>
                <GameStateProvider>
                    <Stack screenOptions={{headerShown: false}}/>
                </GameStateProvider>
            </UserStateProvider>
        </GameListProvider>
    )
}
