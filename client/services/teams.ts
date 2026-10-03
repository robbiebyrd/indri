import {Game, Team} from "@/models/models";

/** The team that lists slot `playerId` among its pre-declared slots. */
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

