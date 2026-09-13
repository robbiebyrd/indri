// EXPO_PUBLIC_API_URL is a ws:// URL because WebSocket was once the only
// transport. Every other channel hangs off the SAME origin — the server mounts
// them all on one mux (internal/entrypoints/http) — so they are derived here
// rather than added as three more environment variables nobody would remember
// to keep in step.
//
// This is the one derivation in the client. webrtc-signal.ts used to carry its
// own copy; with three channels a copy per transport would eventually disagree
// about the scheme or the path, and the failure mode is a channel that quietly
// never connects, which a supervisor reads as "unavailable here" rather than
// "misconfigured".

/** The server routes, as mounted in Go. Keep these in step with the server. */
const RTC_OFFER_PATH = "/rtc/offer"   // internal/transport/webrtc/webrtc.go
const EVENTS_PATH = "/events"         // internal/transport/sse/sse.go
const API_PREFIX = "/api"             // internal/transport/rest/rest.go

export interface Endpoints {
    /** The WebSocket url, used exactly as configured. */
    websocket: string
    /** POST target for the WebRTC SDP offer. */
    rtcOffer: string
    /** The SSE stream. A session token must be appended by the caller. */
    events: string
    /** Prefix for the REST action routes: `${api}/<action>`. */
    api: string
}

export function endpoints(apiUrl: string): Endpoints {
    const origin = httpOrigin(apiUrl)

    return {
        websocket: apiUrl,
        rtcOffer: origin + RTC_OFFER_PATH,
        events: origin + EVENTS_PATH,
        api: origin + API_PREFIX,
    }
}

/**
 * The http(s) origin of the configured ws(s) base, with no path, query or
 * fragment — the three routes above are absolute on the server, so anything
 * the base carries beyond its origin is irrelevant to them.
 *
 * Throws rather than falling back. An empty or malformed base is missing
 * config, and a silent fallback would resolve against the page origin and make
 * the app look like a broken server instead.
 */
function httpOrigin(apiUrl: string): string {
    let u: URL
    try {
        u = new URL(apiUrl)
    } catch {
        throw new Error(
            `EXPO_PUBLIC_API_URL is not a valid url (${JSON.stringify(apiUrl)}). ` +
            "Copy client/.env.example to client/.env and restart Metro — " +
            "EXPO_PUBLIC_* values are inlined at build time.",
        )
    }

    u.protocol = u.protocol === "wss:" ? "https:" : "http:"
    u.pathname = ""
    u.search = ""
    u.hash = ""

    // `origin` rather than toString(): the latter re-adds a trailing slash for
    // an empty pathname, which would produce "//events".
    return u.origin
}
