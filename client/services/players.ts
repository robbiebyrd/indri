import type {Game, Player} from "@/models/models";

/**
 * The slot `userId` holds in `game`, if any.
 *
 * `game.players` is keyed by slot id, not user id: every slot exists from the
 * moment the game is created, and `player.userId` names its holder (`""` while
 * it is free). So a user is found by the id they hold, never by key.
 */
export function findSlot(
    game: Game | undefined,
    userId: string | undefined,
): { slotId: string, player: Player } | undefined {
    if (!game?.players || !userId) {
        return undefined
    }

    for (const [slotId, player] of Object.entries(game.players)) {
        if (player.userId === userId) {
            return {slotId, player}
        }
    }

    return undefined
}

/**
 * Whether `userId` holds the game's host slot. This is the same flag the
 * server's host-only actions authorize against, so it is a UX gate: a
 * non-host who calls one anyway is rejected server-side.
 */
export function isHost(game: Game | undefined, userId: string | undefined): boolean {
    return findSlot(game, userId)?.player.host === true
}
