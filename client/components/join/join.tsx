import {Button} from 'react-native'
import {useEffect, useState} from "react"
import Select from 'react-select'
import {MessageHandler} from "@/services/message-handler";
import {useGameList} from "@/providers/game-list/use-game-list";
import GameListRefreshButton from "@/components/join/listRefresh";

export type GameRefreshProps = {
    ws: MessageHandler
}

export default function Join({ws}: GameRefreshProps) {
    const [gameCode, setGameCode] = useState<string>()
    const [teamID, setTeamID] = useState<string>()

    const {gameList} = useGameList()

    const gameListOptions = gameList?.filter(Boolean).map((game) => (
        {value: game.code, label: game.code}
    ))

    const teamOptions = gameList?.flatMap((game) => {
        return game?.teams.map((team) => {
            if (game.code === gameCode && !team.full) {
                return {value: team.name, label: team.name}
            }
        })
    }).filter(Boolean)

    useEffect(() => {
        ws.send({"action": "inquire", "inquiryType": "game", "inquiry": "availableGames"})
    }, [])

    return (
        <>
            <Select
                options={gameListOptions}
                onChange={(data) => {
                    setGameCode(data?.value)
                    setTeamID(undefined)
                }}/>
            <Select
                options={teamOptions}
                key={gameCode}
                onChange={(data) => setTeamID(data?.value)}
            />
            <GameListRefreshButton ws={ws}/>
            <Button
                title={"Join"}
                onPress={() => ws?.send(
                    {"action": "join", "code": gameCode, "teamId": teamID}
                )}
            />
        </>
    )
}

