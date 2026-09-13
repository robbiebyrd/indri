// The stored bearer token, cached in memory so it can be read SYNCHRONOUSLY.
//
// That is the whole reason this file exists. On a failover the supervisor
// flushes its queued sends the instant a channel opens, and `reconnect` has to
// be on the wire before them — a connection with no session rejects everything
// else. An AsyncStorage read at that moment lands a turn of the event loop
// late, which is a turn too late.
//
// It is also what makes SSE a conditional candidate: that channel authenticates
// at its handshake, so the supervisor has to be able to ask "is there a token
// yet?" while building its candidate list, without awaiting.
import AsyncStorage from "@react-native-async-storage/async-storage"

// The key predates this file; the WebSocket protocol calls the token
// "sessionId" on the wire (see the sessionId overload in CLAUDE.md).
const KEY = "sessionId"

let cached: string | null = null

/**
 * The token as last seen, or null. Never awaits, and never reads storage — it
 * answers with whatever `load` or `remember` last put here.
 */
export function currentToken(): string | null {
    return cached
}

/** Reads the persisted token into the cache. Returns what it found. */
export async function loadToken(): Promise<string | null> {
    const stored = await AsyncStorage.getItem(KEY)

    // An empty string is not a token; normalising here means every caller can
    // test for null alone.
    cached = stored === "" ? null : stored

    return cached
}

/**
 * Records a token issued by `login` or `reconnect`, in memory first so it is
 * usable on the very next connect, then persisted for the next app start.
 */
export async function rememberToken(token: string): Promise<void> {
    cached = token

    // NEVER LOG THE TOKEN. It is a bearer credential: anything holding it can
    // act as this user on any transport.
    await AsyncStorage.setItem(KEY, token)
}
