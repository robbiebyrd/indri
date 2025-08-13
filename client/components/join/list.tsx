import {MessageHandler} from "@/services/message-handler";
import {useGameList} from "@/providers/game-list/use-game-list";

export type GameListProps = {
    ws: MessageHandler
}

export default function GameList({ws}: GameListProps) {

    const {gameList} = useGameList()
    return <>
        <div>{gameList?.length || 0} Games Available</div>
        {gameList?.map((game) =>
            <div key={game.code}>{game.code} {game.full ? "" : "+"}</div>)
        }
    </>
}
