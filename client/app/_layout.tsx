import {Stack} from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import {GameStateProvider} from "@/providers/game-state/game-state-provider";
import {UserStateProvider} from "@/providers/user-state/user-state-provider";

SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
    return (
        <GameStateProvider>
            <UserStateProvider>
                <Stack screenOptions={{headerShown: false}}/>
            </UserStateProvider>
        </GameStateProvider>
    )
}
