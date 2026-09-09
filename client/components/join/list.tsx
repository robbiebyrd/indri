import {Text, View} from 'react-native'
import {MessageHandler} from "@/services/message-handler";
import {useGameList} from "@/providers/game-list/use-game-list";

export type GameListProps = {
    ws: MessageHandler
}

export default function GameList({ws}: GameListProps) {

    const {gameList} = useGameList()
    return <View>
        <Text>{gameList?.length || 0} Games Available</Text>
        {gameList?.map((game) =>
            <Text key={game.code}>{game.code} {game.full ? "" : "+"}</Text>)
        }
    </View>
}
