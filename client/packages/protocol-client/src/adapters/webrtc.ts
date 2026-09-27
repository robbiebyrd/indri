import {BaseTransportClient, resolveFetch, resolveGlobal, splitBaseURL, toPayload} from "../transport.ts"
import type {FetchLike, Payload} from "../transport.ts"

type SessionDescription = {type: string, sdp: string}

type Listener = {addEventListener(type: string, fn: (e: any) => void): void}

/** The subset of RTCDataChannel used here; browsers and react-native-webrtc both provide it. */
interface DataChannelLike extends Listener {
    binaryType: string
    send(data: string | ArrayBufferView): void
}

/** The subset of RTCPeerConnection used here. */
interface PeerConnectionLike extends Listener {
    readonly iceGatheringState: string
    readonly connectionState: string
    readonly localDescription: SessionDescription | null
    createDataChannel(label: string): DataChannelLike
    createOffer(): Promise<SessionDescription>
    setLocalDescription(d: SessionDescription): Promise<void>
    setRemoteDescription(d: SessionDescription): Promise<void>
    close(): void
}

export type PeerConnectionConstructor = new (config: {iceServers?: {urls: string | string[]}[]}) => PeerConnectionLike

export type WebRtcConfig = {
    /** Base HTTP URL of the server, e.g. http://localhost:5002. */
    url: string
    /** fetch implementation for the signaling POST; defaults to the global. */
    fetch?: FetchLike
    /**
     * RTCPeerConnection implementation; defaults to the global, which
     * browsers have. On React Native pass react-native-webrtc's.
     */
    RTCPeerConnection?: PeerConnectionConstructor
    iceServers?: {urls: string | string[]}[]
}

// Bounds ICE gathering so an unreachable STUN server can't stall the
// connection; the offer then carries whatever was gathered.
const GATHER_TIMEOUT_MS = 5000

/**
 * WebRTC data channel. Signaling is one POST carrying a complete offer (no
 * trickle ICE); after that, messages flow over the channel.
 */
export class WebRtcTransportClient extends BaseTransportClient {
    private readonly base: string
    private readonly query: string
    private readonly fetch: FetchLike
    private readonly PeerConnection: PeerConnectionConstructor
    private readonly iceServers?: {urls: string | string[]}[]
    private pc?: PeerConnectionLike
    private dc?: DataChannelLike

    constructor(config: WebRtcConfig) {
        super()
        const {root, query} = splitBaseURL(config.url)
        this.base = root
        this.query = query
        this.iceServers = config.iceServers
        this.fetch = resolveFetch(config.fetch, "Pass a fetch implementation in the config.")
        this.PeerConnection = resolveGlobal(
            config.RTCPeerConnection,
            "RTCPeerConnection",
            "On React Native, install react-native-webrtc (it needs a development build, not Expo Go) " +
            "and pass its RTCPeerConnection in the config.",
        )
    }

    protected start() {
        void this.negotiate()
    }

    private async negotiate() {
        try {
            const pc = new this.PeerConnection(this.iceServers ? {iceServers: this.iceServers} : {})
            this.pc = pc

            // The channel must exist before the offer so the offer includes it.
            const dc = pc.createDataChannel("indri")
            this.dc = dc
            dc.binaryType = "arraybuffer"
            dc.addEventListener("open", () => this.opened())
            dc.addEventListener("message", (e) => this.emitMessage(toPayload(e.data)))
            dc.addEventListener("close", () => this.ended("data channel closed"))

            pc.addEventListener("connectionstatechange", () => {
                if (pc.connectionState === "failed" || pc.connectionState === "closed") {
                    this.ended(`peer connection ${pc.connectionState}`)
                }
            })

            await pc.setLocalDescription(await pc.createOffer())
            await gatheringComplete(pc)

            const offer = pc.localDescription
            if (!offer) {
                throw new Error("no local description after gathering")
            }

            const res = await this.fetch(`${this.base}/webrtc/offer${this.query}`, {
                method: "POST",
                headers: {"Content-Type": "application/json"},
                body: JSON.stringify({type: offer.type, sdp: offer.sdp}),
            })

            if (!res.ok) {
                this.ended(`offer refused (${res.status})`)
                return
            }

            await pc.setRemoteDescription(await res.json())
        } catch (e) {
            this.ended(e instanceof Error ? e.message : String(e))
        }
    }

    protected transmit(data: Payload) {
        this.dc?.send(data)
    }

    protected teardown() {
        this.pc?.close()
    }
}

function gatheringComplete(pc: PeerConnectionLike): Promise<void> {
    if (pc.iceGatheringState === "complete") {
        return Promise.resolve()
    }

    return new Promise((resolve) => {
        const timer = setTimeout(resolve, GATHER_TIMEOUT_MS)
        pc.addEventListener("icegatheringstatechange", () => {
            if (pc.iceGatheringState === "complete") {
                clearTimeout(timer)
                resolve()
            }
        })
    })
}
