import {Stack} from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import {GestureHandlerRootView} from "react-native-gesture-handler";
import {GameStateProvider} from "@/providers/game-state/game-state-provider";
import {UserStateProvider} from "@/providers/user-state/user-state-provider";
import {GameListProvider} from "@/providers/game-list/game-list-provider";

SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
    return (
        // GestureHandlerRootView must wrap the entire app on every platform
        // (including web) for react-native-gesture-handler to function.
        <GestureHandlerRootView style={{flex: 1}}>
            <GameListProvider>
                <UserStateProvider>
                    <GameStateProvider>
                        <Stack screenOptions={{headerShown: false}}/>
                    </GameStateProvider>
                </UserStateProvider>
            </GameListProvider>
        </GestureHandlerRootView>
    )
}
