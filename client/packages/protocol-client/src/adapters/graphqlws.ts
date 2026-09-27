import {base64Decode, base64Encode} from "../codec.ts"
import {BaseTransportClient, closeSocket, resolveGlobal} from "../transport.ts"
import type {Payload, WebSocketConstructor, WebSocketLike} from "../transport.ts"

export type GraphQLWsConfig = {
    /** Full URL of the server's GraphQL endpoint, e.g. ws://localhost:5002/graphql. */
    url: string
    /** WebSocket implementation; defaults to the global. */
    WebSocket?: WebSocketConstructor
}

const SUBPROTOCOL = "graphql-transport-ws"
const EVENTS_ID = "events"
const EVENTS_QUERY = "subscription IndriEvents { indriEvents }"
const SEND_QUERY = "mutation Send($message: String, $b64: String) { send(message: $message, b64: $b64) }"

/**
 * graphql-transport-ws over one socket: an IndriEvents subscription carries
 * server messages, and each client message is a Send operation. Framing only;
 * the server executes no GraphQL.
 */
export class GraphQLWsTransportClient extends BaseTransportClient {
    private readonly url: string
    private readonly WS: WebSocketConstructor
    private ws?: WebSocketLike
    private sends = 0

    constructor(config: GraphQLWsConfig) {
        super()
        this.url = config.url
        this.WS = resolveGlobal(config.WebSocket, "WebSocket", "Pass a WebSocket implementation in the config.")
    }

    protected start() {
        const ws = new this.WS(this.url, SUBPROTOCOL)
        this.ws = ws

        ws.onopen = () => this.write({type: "connection_init"})
        ws.onmessage = (e) => this.frame(e.data)
        ws.onerror = (e) => this.emitError(e)
        ws.onclose = (e) => this.ended(e.reason || `socket closed (${e.code ?? "no code"})`)
    }

    private frame(raw: unknown) {
        let m: {id?: string, type?: string, payload?: any}
        try {
            m = JSON.parse(String(raw))
        } catch {
            this.emitError(new Error("unparseable graphql-transport-ws message"))
            return
        }

        switch (m.type) {
            case "connection_ack":
                this.write({
                    id: EVENTS_ID,
                    type: "subscribe",
                    payload: {operationName: "IndriEvents", query: EVENTS_QUERY},
                })
                this.opened()
                break
            case "next":
                if (m.id === EVENTS_ID) {
                    this.event(m.payload?.data?.indriEvents)
                }
                break
            case "complete":
                if (m.id === EVENTS_ID) {
                    this.ended("the server ended the event subscription")
                }
                break
            case "error":
                this.emitError(new Error(`operation ${m.id} failed: ${JSON.stringify(m.payload)}`))
                break
            case "ping":
                this.write({type: "pong"})
                break
        }
    }

    private event(ev: {text?: unknown, b64?: unknown} | undefined) {
        if (typeof ev?.b64 === "string") {
            this.emitMessage(base64Decode(ev.b64))
        } else if (typeof ev?.text === "string") {
            this.emitMessage(ev.text)
        } else {
            this.emitError(new Error("indriEvents frame without text or b64"))
        }
    }

    protected transmit(data: Payload) {
        const variables = typeof data === "string" ? {message: data} : {b64: base64Encode(data)}

        this.write({
            id: `send-${++this.sends}`,
            type: "subscribe",
            payload: {operationName: "Send", query: SEND_QUERY, variables},
        })
    }

    private write(message: object) {
        this.ws?.send(JSON.stringify(message))
    }

    protected teardown() {
        closeSocket(this.ws)
        this.ws = undefined
    }
}
