import {GraphQLWsTransportClient} from "./adapters/graphqlws.ts"
import {SseTransportClient} from "./adapters/sse.ts"
import {WebRtcTransportClient} from "./adapters/webrtc.ts"
import type {PeerConnectionConstructor} from "./adapters/webrtc.ts"
import {WebSocketTransportClient} from "./adapters/websocket.ts"
import type {FetchLike, TransportClient, WebSocketConstructor} from "./transport.ts"

/** The same names the server accepts in INDRI_TRANSPORTS. */
export const TRANSPORT_KINDS = ["ws", "sse", "graphqlws", "webrtc"] as const

export type TransportKind = typeof TRANSPORT_KINDS[number]

export type TransportClientConfig = {
    /**
     * The endpoint for the chosen kind: the WebSocket URL for ws
     * (ws://host:port/ws) and graphqlws (ws://host:port/graphql), or the
     * server's base HTTP URL for sse and webrtc (http://host:port).
     */
    url: string
    WebSocket?: WebSocketConstructor
    fetch?: FetchLike
    RTCPeerConnection?: PeerConnectionConstructor
    iceServers?: {urls: string | string[]}[]
}

/** Builds the client for a transport kind, e.g. one read from configuration. */
export function createTransportClient(kind: string, config: TransportClientConfig): TransportClient {
    switch (kind) {
        case "ws":
            return new WebSocketTransportClient(config)
        case "sse":
            return new SseTransportClient(config)
        case "graphqlws":
            return new GraphQLWsTransportClient(config)
        case "webrtc":
            return new WebRtcTransportClient(config)
        default:
            throw new Error(`unknown transport ${JSON.stringify(kind)}; expected one of ${TRANSPORT_KINDS.join(", ")}`)
    }
}
