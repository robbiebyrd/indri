import {Button, Text, TextInput} from 'react-native'
import {MessageHandler} from "@/services/message-handler";
import {useState} from "react";
import Checkbox from 'expo-checkbox';

export type GameCreateProps = {
    ws: MessageHandler
}

export default function GameCreate({ws}: GameCreateProps) {
    const [gameCode, setGameCode] = useState<string>()
    const [teamID, setTeamID] = useState<string>()
    const [isPrivate, setIsPrivate] = useState(false);

    return <>
        <Text>Game Code: </Text>
        <TextInput onChangeText={setGameCode}/>
        <Text>Team Name: </Text>
        <TextInput onChangeText={setTeamID}/>
        <Text>Private Game? </Text>
        <Checkbox
            value={isPrivate}
            onValueChange={setIsPrivate}
            color={isPrivate ? '#4630EB' : undefined}
        />
        <Button
            title={"Create"}
            onPress={() => ws?.send({
                    "action": "create",
                    "code": gameCode,
                    "teamId": teamID,
                    "private": isPrivate
                }
            )}
        />
    </>
}
