import {Button, StyleSheet, Text, TextInput, View} from 'react-native';
import {useMemo, useState} from "react";
import {deleteJSONKeyByDotPath, updateJSONKeyByDotPath} from "@/app/handler";
import Select from 'react-select';


type User = {
    ID: string
    email: string
    name: string
    displayName: string
    score: number
}

export default function Index() {

    const [gameState, setGameState] = useState<object>()
    const [user, setUser] = useState<User>()
    const [userName, setUserName] = useState<string>()
    const [password, setPassword] = useState<string>()
    const [gameCode, setGameCode] = useState<string>()
    const [teamID, setTeamID] = useState<string>()

    const ws: WebSocket = useMemo(() => {
        return new WebSocket('ws://localhost:5002/ws');
    }, [])

    ws.onmessage = (e: MessageEvent) => {
        console.log(e.data)
        const parsed = JSON.parse(e.data)
        if ("authenticated" in parsed && parsed["authenticated"] == true) {
            setUser(parsed["user"] as User)
            return
        }
        if (("op" in parsed)) {
            let newState = gameState;

            for (const [key, value] of Object.entries(parsed.updated)) {
                newState = updateJSONKeyByDotPath(newState || {}, key, value)
            }

            for (const key of parsed.deleted) {
                newState = deleteJSONKeyByDotPath(newState || {}, key)
            }

            console.log("Setting state")
            setGameState({...newState})
        }

        if ("code" in parsed) {
            setGameState(parsed)
        }

    };

    ws.onerror = (e: Event) => {
        console.log(e);
    };

    ws.onclose = (e: CloseEvent) => {
        console.log(e.code, e.reason);
    };


    const stateDisplay = useMemo(() => {
        return <p>{JSON.stringify(gameState)}</p>
    }, [gameState])

    const options = [
        {value: 'team1', label: 'Team 1'},
        {value: 'team2', label: 'Team 2'},
    ];

    return (
        <View style={styles.container}>
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

            {user && !gameState && (
                <>
                    <TextInput onChangeText={setGameCode}/>
                    <Select options={options} onInputChange={setTeamID}/>
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
