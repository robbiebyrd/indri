import {Game, Team} from "@/models/models";

export const getTeam = (playerId?: string, gameState?: Game): { [key: string]: Team } | undefined => {
    if (!playerId || !gameState) {
        return undefined
    }
    for (const [teamId, team] of Object.entries(gameState?.teams ?? {})) {
        if (team.playerIds.includes(playerId)) {
            let t: { [key: string]: Team } = {}
            t[teamId] = team
            return t
        }
    }

    return undefined
}

