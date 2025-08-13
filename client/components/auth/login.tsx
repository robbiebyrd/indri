import {Button, Text, TextInput} from 'react-native'
import {MessageHandler} from "@/services/message-handler";
import {useState} from "react";

export type LoginProps = {
    onChangeUsername?: (text: string) => void
    onChangePassword?: (text: string) => void
    ws: MessageHandler
}

export default function Login({onChangeUsername, onChangePassword, ws}: LoginProps) {
    const [userName, setUserName] = useState<string>()
    const [password, setPassword] = useState<string>()
    return (
        <>
            <Text>Login</Text>

            <TextInput onChangeText={(text: string) => {
                setUserName(text)
                onChangeUsername?.(text)
            }}/>

            <TextInput onChangeText={(text: string) => {
                setPassword(text)
                onChangePassword?.(text)
            }} secureTextEntry={true}/>

            <Button title={"Login"}
                    onPress={() => ws?.send({
                        "action": "login",
                        "email": userName,
                        "password": password
                    })}
            />
        </>
    )
}
