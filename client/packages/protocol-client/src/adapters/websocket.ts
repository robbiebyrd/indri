import {BaseTransportClient, closeSocket, resolveGlobal, toPayload} from "../transport.ts"
import type {Payload, WebSocketConstructor, WebSocketLike} from "../transport.ts"

export type WebSocketConfig = {
    /** Full URL of the server's WebSocket endpoint, e.g. ws://localhost:5002/ws. */
    url: string
    /** WebSocket implementation; defaults to the global. */
    WebSocket?: WebSocketConstructor
}

/** Plain WebSocket: text frames are text, binary frames are binary. */
export class WebSocketTransportClient extends BaseTransportClient {
    private readonly url: string
    private readonly WS: WebSocketConstructor
    private ws?: WebSocketLike

    constructor(config: WebSocketConfig) {
        super()
        this.url = config.url
        this.WS = resolveGlobal(config.WebSocket, "WebSocket", "Pass a WebSocket implementation in the config.")
    }

    protected start() {
        const ws = new this.WS(this.url)
        this.ws = ws
        ws.binaryType = "arraybuffer"

        ws.onopen = () => this.opened()
        ws.onmessage = (e) => this.emitMessage(toPayload(e.data))
        ws.onerror = (e) => this.emitError(e)
        ws.onclose = (e) => this.ended(e.reason || `socket closed (${e.code ?? "no code"})`)
    }

    protected transmit(data: Payload) {
        this.ws?.send(data)
    }

    protected teardown() {
        closeSocket(this.ws)
        this.ws = undefined
    }
}
