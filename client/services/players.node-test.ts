// The game's players are keyed by slot, not by user: these find a user's own
// slot by the userId it holds. Run with:
//   pnpm test
import test from "node:test";
import assert from "node:assert/strict";

import {findSlot, isHost} from "./players.ts";

const game: any = {
    players: {
        p0: {userId: "u-host", name: "Host", host: true},
        p1: {userId: "u-guest", name: "Guest", host: false},
        p2: {userId: "", name: "", host: false},
    },
};

test("findSlot finds the slot a user holds, by its userId rather than its key", () => {
    const found = findSlot(game, "u-guest");
    assert.ok(found, "the guest holds a slot");
    assert.equal(found.slotId, "p1");
    assert.equal(found.player.name, "Guest");
});

test("findSlot finds nothing for a user who holds no slot, or no user at all", () => {
    assert.equal(findSlot(game, "u-stranger"), undefined);
    assert.equal(findSlot(game, undefined), undefined);
    // A free slot's userId is "", and an empty id must not match it.
    assert.equal(findSlot(game, ""), undefined);
    assert.equal(findSlot(undefined, "u-host"), undefined);
});

test("isHost is true only for the user holding the host slot", () => {
    assert.equal(isHost(game, "u-host"), true);
    assert.equal(isHost(game, "u-guest"), false);
    // A user id that happens to equal a slot key is not that slot's holder.
    assert.equal(isHost(game, "p0"), false);
    assert.equal(isHost(game, undefined), false);
});
