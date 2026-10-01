"""Measure a realistic session, not a single message.

permessage-deflate with context takeover keeps one compression stream per
connection, so the Nth delta is compressed against every delta before it.
A per-message gzip measurement badly understates that. This models a whole
game: 9 moves, each a board cell plus two turn flags.
"""
import json, msgpack, zlib, importlib.util, pathlib

spec = importlib.util.spec_from_file_location(
    "measure", pathlib.Path(".integration/scratch/measure.py"))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

j, mp, fmt_A, fmt_B, fmt_D = m.j, m.mp, m.fmt_A, m.fmt_B, m.fmt_D

CELLS = [(0,0,"X"),(1,1,"O"),(0,1,"X"),(2,2,"O"),(0,2,"X"),
         (1,0,"O"),(2,0,"X"),(1,2,"O"),(2,1,"X")]

def session_deltas():
    out, turn = [], True
    for r, c, mark in CELLS:
        out.append({
            f"stage.scenes.board.data.board.{r}.{c}": mark,
            "teams.Player 1.data.turn": not turn,
            "teams.Player 2.data.turn": turn,
        })
        turn = not turn
    return out

def stream_size(msgs, encode, compress):
    """Total bytes on the wire for a session."""
    if not compress:
        return sum(len(encode(x)) for x in msgs)
    # one deflate stream for the connection, flushed per message:
    # exactly what permessage-deflate with context takeover does
    co = zlib.compressobj(6, zlib.DEFLATED, -15)
    total = 0
    for x in msgs:
        total += len(co.compress(encode(x)) + co.flush(zlib.Z_SYNC_FLUSH))
    return total

deltas = session_deltas()
encA = lambda d: j(fmt_A(d))
encB = lambda d: j(fmt_B(d))
encC = lambda d: mp(fmt_B(d))
encD = lambda d: mp(fmt_D(d))

print("A full 9-move game, total delta bytes for one client:")
print()
print(f"{'encoding':<34}{'raw':>8}{'+deflate':>10}{'vs A raw':>11}")
print("-" * 63)
base = stream_size(deltas, encA, False)
for name, enc in [("A  local today (JSON, dotted)", encA),
                  ("B  short names + pair arrays", encB),
                  ("C  B + MessagePack", encC),
                  ("D  C + positional paths", encD)]:
    raw = stream_size(deltas, enc, False)
    defl = stream_size(deltas, enc, True)
    print(f"{name:<34}{raw:>8}{defl:>10}{100*raw/base-100:>10.0f}%")

print()
a_d = stream_size(deltas, encA, True)
c_d = stream_size(deltas, encC, True)
d_d = stream_size(deltas, encD, True)
print("What each option actually costs on the wire:")
print(f"  A + deflate (one-line change)  : {a_d:>6} bytes")
print(f"  C + deflate (msgpack, no posn) : {c_d:>6} bytes")
print(f"  D + deflate (remote's full fmt): {d_d:>6} bytes")
print()
print(f"  deflate alone gets {100-100*a_d/base:.0f}% off today's format.")
print(f"  going all the way to D buys a further {100-100*d_d/a_d:.0f}% on top of that.")
