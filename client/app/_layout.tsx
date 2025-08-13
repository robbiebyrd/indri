import {Stack} from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import {GameStateProvider} from "@/providers/game-state/game-state-provider";
import {UserStateProvider} from "@/providers/user-state/user-state-provider";
import {GameListProvider} from "@/providers/game-list/game-list-provider";

SplashScreen.preventAutoHideAsync();

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
