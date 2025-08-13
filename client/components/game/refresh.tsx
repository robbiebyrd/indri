import {Button} from 'react-native'
import {MessageHandler} from "@/services/message-handler";

export type GameRefreshProps = {
    ws: MessageHandler
}

export default function GameRefreshButton({ws}: GameRefreshProps) {
    return (
        <Button
            title={"Refresh"}
            onPress={() => ws?.send(
                {"action": "refresh"}
            )}
        />
    )
}
