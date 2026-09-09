import {Button} from 'react-native'
import {useEffect, useState} from "react"
import Select, {SelectOption} from "@/components/display/select";
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

    const gameListOptions: SelectOption[] = (gameList ?? [])
        .filter(Boolean)
        .map((game) => ({value: game.code, label: game.code}))

    const teamOptions: SelectOption[] = (gameList ?? [])
        .filter((game) => game.code === gameCode)
        .flatMap((game) => game.teams)
        .filter((team) => !team.full)
        .map((team) => ({value: team.name, label: team.name}))

    useEffect(() => {
        ws.send({"action": "inquire", "inquiryType": "game", "inquiry": "availableGames"})
    }, [])

    return (
        <>
            <Select
                options={gameListOptions}
                value={gameCode}
                placeholder={"No games available"}
                onChange={(value) => {
                    setGameCode(value)
                    setTeamID(undefined)
                }}/>
            <Select
                options={teamOptions}
                value={teamID}
                placeholder={"Select a game first"}
                onChange={(value) => setTeamID(value)}
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

