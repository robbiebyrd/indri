import {Button} from 'react-native'
import {MessageHandler} from "@/services/message-handler";

export type GameListRefreshButtonProps = {
    ws: MessageHandler
}

export default function GameListRefreshButton({ws}: GameListRefreshButtonProps) {
    return <Button title={"Get Games"} onPress={() => ws?.send(
        {"action": "inquire", "inquiryType": "game", "inquiry": "availableGames"}
    )}/>
}
