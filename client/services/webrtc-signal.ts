// The pure, platform-independent parts of the WebRTC transport (Step 11,
// plans/webrtc-transport.md): the signal envelope, the /rtc/offer URL
// derivation, DataChannel payload normalisation and the non-trickle
// gather-complete wait. None of this touches react-native-webrtc-web-shim.
//
// That split is not stylistic — it is why this file is importable at all
// under `node --experimental-strip-types` (see pnpm test). The shim's own
// internals use extensionless relative imports that only Metro's resolver
// can follow; a plain Node ESM loader fails hard trying to resolve them. If
// these helpers lived in webrtc-transport.ts alongside the shim's top-level
// import, loading ANY export from that file — even one that never touches
// the shim at runtime — would still crash under node's module loader, since
// ES module evaluation has no notion of "only load the part I asked for".
// webrtc-transport.node-test.ts imports from here instead.
import {parseJsonSafely} from "./json.ts"

// How long the transport waits for non-trickle ICE gathering before giving
// up. A hang here is a silent failure with no error anywhere else in the
// stack, per the story — this is what turns it into a loud one.
export const ICE_GATHERING_TIMEOUT_MS = 5000

/**
 * The signal envelope exchanged with POST /rtc/offer — exactly
 * internal/transport/webrtc/signal.go's Signal. `peerId` is unused by this
 * transport (renegotiation, story 041) but round-trips it if present so a
 * future caller isn't blocked on this file.
 */
export interface Signal {
    type: string
    sdp: string
    peerId?: string
}

/** Builds the offer envelope POSTed to /rtc/offer from a local description. */
export function buildOfferSignal(description: {type: string, sdp: string}): Signal {
    return {type: description.type, sdp: description.sdp}
}

/**
 * Parses a signal envelope defensively — malformed input (wrong shape,
 * missing fields) returns `undefined` rather than throwing, matching this
 * codebase's non-throwing parse convention (see layout/schema/layout.ts's
 * parseLayout). The caller decides what a missing answer means; this only
 * validates shape.
 */
export function parseSignal(data: unknown): Signal | undefined {
    if (typeof data !== "object" || data === null) {
        return undefined
    }

    const record = data as Record<string, unknown>
    if (typeof record.type !== "string" || typeof record.sdp !== "string") {
        return undefined
    }

    return {
        type: record.type,
        sdp: record.sdp,
        ...(typeof record.peerId === "string" ? {peerId: record.peerId} : {}),
    }
}

/** Parses a raw JSON response body into a Signal, or undefined if malformed. */
export function parseSignalFromJson(jsonText: string): Signal | undefined {
    return parseSignal(parseJsonSafely<unknown>(jsonText))
}

/**
 * The signalling route is not derived from the WebSocket URL's path — it is
 * a fixed, absolute route on the same origin
 * (internal/transport/webrtc/webrtc.go signalPath) — only the scheme
 * changes: ws(s):// becomes http(s)://.
 */
export function signalUrl(wsUrl: string): string {
    const u = new URL(wsUrl)
    u.protocol = u.protocol === "wss:" ? "https:" : "http:"
    u.pathname = "/rtc/offer"
    u.search = ""
    u.hash = ""
    return u.toString()
}

/**
 * Normalises a DataChannel message payload to a string, which is what
 * ClientTransport.onMessage hands its consumer.
 *
 * This is a REAL cross-platform trap, not boilerplate: react-native-webrtc
 * bridges binary DataChannel payloads across the JS bridge as base64, while
 * the browser gives an ArrayBuffer (with binaryType 'arraybuffer', set by
 * WebRTCTransport) or a Blob (the default binaryType, which is why it is
 * always overridden). Both implementations converge on ArrayBuffer by the
 * time `onmessage` fires — react-native-webrtc's own RTCDataChannel decodes
 * its base64 wire format into an ArrayBuffer before dispatching the event —
 * so this one function is the single place that turns whichever of the two
 * shapes arrives into the string form every consumer downstream expects.
 */
export function normaliseMessage(data: string | ArrayBuffer): string {
    if (typeof data === "string") {
        return data
    }

    return new TextDecoder().decode(data)
}

/**
 * The slice of RTCPeerConnection the gather-complete wait needs. Isolating it
 * behind this interface is what lets webrtc-transport.node-test.ts drive the
 * wait with a plain object — the client's node test runner cannot construct
 * a real PeerConnection at all.
 */
export interface GatherableConnection {
    iceGatheringState: string
    addEventListener(type: "icegatheringstatechange", listener: () => void): void
    removeEventListener(type: "icegatheringstatechange", listener: () => void): void
}

/**
 * Resolves once `pc.iceGatheringState` is "complete" — the non-trickle
 * contract the server requires: it returns one complete SDP answer with
 * candidates already gathered, and there is no channel to stream late
 * candidates over afterwards. Gathering fully before the POST is therefore
 * not an optimisation, it is the protocol.
 *
 * Bounded by `timeoutMs`: a hang here would otherwise be a silent failure
 * with no error anywhere else in the stack.
 */
export function waitForIceGatheringComplete(
    pc: GatherableConnection,
    timeoutMs: number = ICE_GATHERING_TIMEOUT_MS,
): Promise<void> {
    if (pc.iceGatheringState === "complete") {
        return Promise.resolve()
    }

    return new Promise<void>((resolve, reject) => {
        const onChange = () => {
            if (pc.iceGatheringState === "complete") {
                cleanup()
                resolve()
            }
        }

        const timer = setTimeout(() => {
            cleanup()
            reject(new Error(`ice gathering did not complete within ${timeoutMs}ms`))
        }, timeoutMs)

        function cleanup() {
            clearTimeout(timer)
            pc.removeEventListener("icegatheringstatechange", onChange)
        }

        pc.addEventListener("icegatheringstatechange", onChange)
    })
}
