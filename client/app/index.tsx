import {Button, StyleSheet, Text, TextInput, View} from 'react-native';
import {useEffect, useMemo, useState} from "react";
import Select from 'react-select';
import {GameStateParser} from "@/app/ordered_list";


declare interface Game {
    code?: string;
    teams?: { [key: string]: Team };
    players?: { [key: string]: Player };
    stage?: Stage;
    data?: { [key: string]: any };
    privateData?: { [key: string]: any };
    playerData?: { [key: string]: any };
    updatedAt?: string;
    createdAt?: string;
}

declare interface Stage {
    currentScene: string;
    sceneOrder: string[];
    scenes?: { [key: string]: Scene };
    data?: { [key: string]: any };
    privateData?: { [key: string]: any };
    playerData?: { [key: string]: { [key: string]: any } };
}

declare interface Scene {
    data?: { [key: string]: any };
    privateData?: { [key: string]: any };
    playerData?: { [key: string]: { [key: string]: any } };
}

declare interface Player {
    name: string;
    score: number;
    connected: boolean;
    host: boolean;
    controller: boolean;
    data?: { [key: string]: any };
    privateData?: { [key: string]: any };
}

declare interface Team {
    name: string;
    playerIds: string[];
    data?: { [key: string]: any };
    privateData?: { [key: string]: any };
    playerData?: { [key: string]: { [key: string]: any } };
}

declare interface User {
    ID: string
    email: string
    name: string
    displayName: string
    score: number
}

declare interface UpdateMessage {
    id: string
    ts: string
    op: string
    type: string
    updated: object
    removed: string[]
}

export default function Index() {
    const [gameState, setGameState] = useState<Game | undefined>(undefined)
    const [user, setUser] = useState<User>()
    const [userName, setUserName] = useState<string>()
    const [password, setPassword] = useState<string>()
    const [gameCode, setGameCode] = useState<string>()
    const [teamID, setTeamID] = useState<string>()

    const stateList: GameStateParser<Game> = useMemo(() => {
        return new GameStateParser<Game>()
    }, [])

    const ws: WebSocket = useMemo(() => {
        return new WebSocket(process.env.EXPO_PUBLIC_API_URL || "");
    }, [])

    useEffect(() => {
        const intervalId = setInterval(() => {
            setGameState({...stateList.current()})
        }, 500);

        return () => clearInterval(intervalId);
    }, [gameState]);

    const keyframe = (parsed: Game) => {
        if (parsed.updatedAt != undefined) {
            stateList.set(parsed, new Date(parsed.updatedAt))
        }
        setGameState(parsed)
    }

    ws.onmessage = (e: MessageEvent) => {
        const parsed = JSON.parse(e.data)
        console.log(parsed)
        if ("authenticated" in parsed && parsed["authenticated"] == true) {
            setUser(parsed["user"] as User)
            return
        } else if ("op" in parsed && "id" in parsed && "ts" in parsed && "type" in parsed) {
            const updateMsg = parsed as UpdateMessage;
            console.log("updateMsg", updateMsg)
            stateList.update(updateMsg, new Date(updateMsg.ts))
            return
        } else if ("code" in parsed && "id" in parsed) {
            keyframe(parsed)
            return
        } else {
            console.log(parsed)
        }
    };

    ws.onerror = (e: Event) => {
        console.log(e);
    };

    ws.onclose = (e: CloseEvent) => {
        console.log(e.code, e.reason);
    };


    const stateDisplay = useMemo(() => {
        return <>
            <p>{JSON.stringify(gameState)}</p>
        </>
    }, [gameState])

    const options = [
        {value: 'team1', label: 'Team 1'},
        {value: 'team2', label: 'Team 2'},
    ];

    return (
        <View style={styles.container}>
            <Text style={styles.text}>
                <>statelist:
                    {JSON.stringify(stateList.getHeap())}</>
            </Text>
            {user ? (
                <Text style={styles.text}>
                    Welcome {user?.displayName || user?.name || "Not Authenticated"}
                </Text>
            ) : (
                <>
                    <Text style={styles.text}>Login</Text>
                    <TextInput onChangeText={setUserName}/>
                    <TextInput onChangeText={setPassword} secureTextEntry={true}/>
                </>
            )}

            <Text style={styles.text}>
                {stateDisplay}
            </Text>

            {!user && (
                <Button title={"Login"}
                        onPress={() => ws?.send(`{"action": "login", "email": "${userName}", "password": "${password}"}`)}/>
            )}

            {user && "id" in user && gameState && !("id" in gameState) && (
                <>
                    <TextInput onChangeText={setGameCode}/>
                    <Select options={options} onChange={(data) => setTeamID(data?.value)}/>
                    <Button title={"Join"}
                            onPress={() => ws?.send(`{"action": "join", "code": "${gameCode}", "teamId": "${teamID}"}`)}/>
                </>
            )}
        </View>
    );
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        backgroundColor: '#25292e',
        alignItems: 'center',
        justifyContent: 'center',
    },
    text: {
        color: '#fff',
    },
});
