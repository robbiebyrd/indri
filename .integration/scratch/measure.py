"""Measure what each layer of the compact delta actually buys, on real data.

Layers, applied cumulatively:
  A  local today      JSON, long field names, dotted-string paths, map-shaped `updated`
  B  short names      JSON, short field names (o/t/u/r), pair-array `u`/`r`
  C  + MessagePack    same shape as B, msgpack framing
  D  + positional     msgpack, object-key path segments replaced by sorted-sibling index

Only D forces the layout-frame subsystem and breaks escaped-key paths.
"""
import json, msgpack, gzip

cfg = json.load(open('example/tictactoe/config.json'))

# ---- build a game document the way a game is stamped from the script -------
def build_game():
    g = {
        "_id": "68f3c2a1b4e5d6f7a8b9c0d1",
        "code": "KJ7P",
        "version": 17,
        "createdAt": "2026-10-01T14:00:00Z",
        "updatedAt": "2026-10-01T14:03:12Z",
        "private": False,
        "teams": json.loads(json.dumps(cfg["teams"])),
        "stage": json.loads(json.dumps(cfg["stage"])),
        "data": json.loads(json.dumps(cfg.get("data") or {})),
        "players": {},
    }
    # four players, the realistic party-game case
    for i, (uid, name, team) in enumerate([
        ("u_7f3a9c21", "Robbie", "Player 1"),
        ("u_2b8e4d10", "Sam", "Player 2"),
        ("u_9c1f7a33", "Alex", "Player 1"),
        ("u_4d6b2e88", "Jo", "Player 2"),
    ]):
        g["players"][uid] = {
            "userId": uid, "name": name, "teamId": team,
            "host": i == 0, "connected": True,
            "data": {}, "playerData": {},
        }
    for t in g["teams"].values():
        t.setdefault("playerIds", [])
    return g

game = build_game()

# ---- the three message kinds that actually travel ------------------------
# 1. a move delta: one board cell plus the two turn flags
move_delta = {
    "stage.scenes.board.data.board.1.2": "X",
    "teams.Player 1.data.turn": False,
    "teams.Player 2.data.turn": True,
}
# 2. a join delta: a whole new player object appears
join_delta = {
    "players.u_4d6b2e88": game["players"]["u_4d6b2e88"],
    "teams.Player 2.playerIds.1": "u_4d6b2e88",
}
# 3. the keyframe: the whole sanitized game
keyframe = game


def j(o):   # compact JSON, as a transport would send it
    return json.dumps(o, separators=(',', ':'), sort_keys=False).encode()

def mp(o):
    return msgpack.packb(o, use_bin_type=True)

def gz(b):
    return gzip.compress(b, 6)

# ---- positional encoding (remote's EncodePath, faithfully) ---------------
def build_posmap(obj, prefix="", out=None):
    if out is None: out = {}
    if not isinstance(obj, dict): return out
    for i, k in enumerate(sorted(obj.keys())):
        p = f"{prefix}.{k}" if prefix else k
        out[p] = i
        if isinstance(obj[k], dict):
            build_posmap(obj[k], p, out)
    return out

posmap = build_posmap(game)

def encode_path(path, schema, pm):
    segs = path.split(".")          # remote splits on a plain dot
    res, cur = [], schema
    for i, seg in enumerate(segs):
        partial = ".".join(segs[:i+1])
        if isinstance(cur, dict):
            res.append(pm[partial] if partial in pm else seg)
            cur = cur.get(seg)
        else:
            try:
                n = int(seg); res.append(n)
                cur = cur[n] if isinstance(cur, list) and n < len(cur) else None
            except (ValueError, TypeError):
                res.append(seg); cur = None
    return res

def fmt_A(delta):
    return {"id": game["_id"], "op": "update", "ts": "2026-10-01T14:03:12.482Z",
            "type": "game", "updated": delta, "removed": []}

def fmt_B(delta):
    return {"id": game["_id"], "o": 1, "t": "2026-10-01T14:03:12.482Z",
            "type": "game", "u": [[k, v] for k, v in delta.items()]}

def fmt_D(delta):
    return {"id": game["_id"], "o": 1, "t": "2026-10-01T14:03:12.482Z",
            "type": "game",
            "u": [[encode_path(k, game, posmap), v] for k, v in delta.items()]}

rows = []
for name, delta in [("move delta", move_delta), ("join delta", join_delta)]:
    a, b = j(fmt_A(delta)), j(fmt_B(delta))
    c, d = mp(fmt_B(delta)), mp(fmt_D(delta))
    rows.append((name, len(a), len(b), len(c), len(d),
                 len(gz(a)), len(gz(d))))

# keyframe: A/B are the same shape (a whole document, not a delta)
kf_json, kf_mp = j({"sv": "a1b2", "game": keyframe}), mp({"sv": "a1b2", "game": keyframe})
# remote pulls layout out of the keyframe onto its own frame
kf_nolayout = json.loads(json.dumps(keyframe)); kf_nolayout["data"].pop("layout", None)
kf_split_mp = mp({"sv": "a1b2", "game": kf_nolayout})
layout_mp = mp({"o": 4, "v": "deadbeef", "data": keyframe["data"]["layout"]})

print(f"{'message':<14}{'A json':>9}{'B short':>9}{'C mpack':>9}{'D posn':>9}"
      f"{'A+gzip':>9}{'D+gzip':>9}")
print("-" * 68)
for n, a, b, c, d, ag, dg in rows:
    print(f"{n:<14}{a:>9}{b:>9}{c:>9}{d:>9}{ag:>9}{dg:>9}")

print()
print(f"keyframe json (whole doc)          : {len(kf_json):>7}")
print(f"keyframe msgpack                   : {len(kf_mp):>7}")
print(f"keyframe msgpack, layout split out : {len(kf_split_mp):>7}")
print(f"  + layout frame (sent once)       : {len(layout_mp):>7}")
print(f"keyframe json + gzip               : {len(gz(kf_json)):>7}")
print(f"keyframe msgpack + gzip            : {len(gz(kf_mp)):>7}")
print()
print("path shapes for the move delta:")
for k in move_delta:
    print(f"  dotted     {k!r}")
    print(f"  positional {encode_path(k, game, posmap)}")
