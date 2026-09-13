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

// The signalling route used to be derived here. It now comes from
// endpoints.ts alongside the SSE and REST routes: three channels deriving
// their own URLs would eventually disagree about the scheme or the path.

// A chunk frame's first byte. internal/transport/webrtc/conn.go's
// sendChunked prefixes every chunk with this exact byte, chosen because a
// JSON text's first non-whitespace byte is always '{' -- every game message
// this transport ever carries is JSON, so a NUL byte can never legitimately
// open one. That is what lets accept() below tell a chunk from an ordinary
// message with a single byte read, with no ambiguity and no need to parse
// the payload to find out.
const CHUNK_MARKER = 0

// The second byte of a chunk frame: whether more chunks follow. There is no
// sequence number -- see ChunkReassembler's comment for why one in-flight
// sequence per connection is all this ever needs to track.
const CHUNK_FLAG_FINAL = 1
const CHUNK_HEADER_BYTES = 2

// Bounds a sequence that never sends its final chunk -- a bug on the server,
// or a corrupted/hostile stream -- so it cannot hold this buffer forever on
// a connection that stays open. Comfortably above any real keyframe; a
// caller building a reassembler for a real connection should leave this at
// its default, tests use a smaller bound to stay fast and deterministic.
const DEFAULT_MAX_REASSEMBLY_BYTES = 64 * 1024 * 1024

/**
 * Reassembles a chunked DataChannel payload (internal/transport/webrtc/conn.go's
 * sendChunked) back into the single message it was split from.
 *
 * One instance is owned per connection (WebRTCTransport creates a fresh one
 * in connect() and drops the reference in close()), which is what keeps a
 * connection closing mid-sequence from leaking its partial buffer: once the
 * transport lets go of the reassembler, nothing else in this module holds a
 * reference to it, and there is no static/module-level table keyed by
 * connection or peer id for a stale sequence to survive in.
 *
 * A chunked sequence can never interleave with another message on the same
 * connection: sendChunked's Go-side comment explains why the server never
 * writes anything else to the wire until every chunk of one sequence has
 * gone out. That is what lets this reassembler track exactly one in-flight
 * sequence with no id to disambiguate it from another.
 *
 * Chunks also carry no sequence number: reassembly relies on the "game"
 * DataChannel being reliable and ordered, which is its default (created with
 * no RTCDataChannelInit in WebRTCTransport.connect). If that channel is ever
 * made unreliable or unordered, this breaks.
 */
export class ChunkReassembler {
    private readonly maxBytes: number
    private chunks: Uint8Array[] = []
    private bufferedBytes = 0

    constructor(maxBytes: number = DEFAULT_MAX_REASSEMBLY_BYTES) {
        this.maxBytes = maxBytes
    }

    /**
     * Feeds one raw DataChannel payload into the reassembler. Returns the
     * complete message's bytes once its final chunk arrives, the bytes
     * unchanged if data was never chunked at all (criterion 4), or
     * undefined while a sequence is still in progress.
     */
    accept(data: Uint8Array): Uint8Array | undefined {
        if (data.length === 0 || data[0] !== CHUNK_MARKER) {
            return data
        }

        const final = data[1] === CHUNK_FLAG_FINAL
        const payload = data.subarray(CHUNK_HEADER_BYTES)

        this.chunks.push(payload)
        this.bufferedBytes += payload.length

        if (this.bufferedBytes > this.maxBytes) {
            // A sequence that never completes must not hold this buffer
            // forever even on a connection that stays open -- reset before
            // throwing so the next message (chunked or not) starts clean.
            this.reset()
            throw new Error(`webrtc: chunked message exceeded ${this.maxBytes} bytes without a final chunk`)
        }

        if (!final) {
            return undefined
        }

        const complete = new Uint8Array(this.bufferedBytes)
        let offset = 0
        for (const chunk of this.chunks) {
            complete.set(chunk, offset)
            offset += chunk.length
        }

        this.reset()

        return complete
    }

    private reset(): void {
        this.chunks = []
        this.bufferedBytes = 0
    }
}

/**
 * Normalises a DataChannel message payload to a string, which is what
 * ClientTransport.onMessage hands its consumer, reassembling chunk sequences
 * along the way (criterion 5: chunk normalisation lives in this single place
 * binary payloads are already normalised, not bolted on beside it).
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
 *
 * A string payload passes straight through without ever touching
 * reassembler: the Go transport's rtcConn always writes with Send([]byte),
 * never SendText, so chunk framing is only ever binary -- a string can never
 * carry it.
 *
 * Returns undefined while reassembler is still buffering a chunk sequence;
 * the caller must not treat that as a message to hand upward.
 */
export function normaliseMessage(data: string | ArrayBuffer, reassembler: ChunkReassembler): string | undefined {
    if (typeof data === "string") {
        return data
    }

    const complete = reassembler.accept(new Uint8Array(data))
    if (!complete) {
        return undefined
    }

    return new TextDecoder().decode(complete)
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
