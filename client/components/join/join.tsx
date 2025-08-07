import {Button, Text, TextInput} from 'react-native'
import {useState} from "react"
import Select from 'react-select'
import {MessageHandler} from "@/services/message-handler";

export type GameRefreshProps = {
    ws: MessageHandler
}


export default function Join({ws}: GameRefreshProps) {
    const [gameCode, setGameCode] = useState<string>()
    const [teamID, setTeamID] = useState<string>()

    return (
        <>
            <Text>Game Code</Text>
            <TextInput onChangeText={setGameCode}/>
            <Select options={[
                {value: 'team1', label: 'Team 1'},
                {value: 'team2', label: 'Team 2'},
            ]} onChange={(data) => setTeamID(data?.value)}/>
            <Button title={"Join"}
                    onPress={() => ws?.send(`{"action": "join", "code": "${gameCode}", "teamId": "${teamID}"}`)}/>
        </>
    )
}

