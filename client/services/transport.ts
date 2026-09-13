// Mirrors the server's transport.Transport split (internal/transport) so
// MessageHandler stops hardcoding WebSocket. A second transport (WebRTC, per
// plans/webrtc-transport.md Step 11) plugs in here without MessageHandler or
// the parsers array changing.
//
// `onOpen` sits alongside the four the plan names because MessageHandler's
// own `onOpen`/`send`-before-open contract (see message-handler.node-test.ts)
// depends on knowing when the underlying connection becomes usable, and that
// notion is transport-specific — a WebSocket has `readyState`, WebRTC will
// have its own equivalent.
export interface ClientTransport {
    connect(url: string): void
    send(message: object): void
    onMessage(handler: (data: string) => void): void
    onOpen(handler: () => void): void
    close(): void
}

export class WebSocketTransport implements ClientTransport {
    private ws?: WebSocket = undefined

    connect(url: string): void {
        this.ws = new WebSocket(url)

        //TODO: Handle errors appropriately.
        this.ws.onerror = (e: Event) => {
            console.log(e)
        }

        //TODO: Handle reconnects
        this.ws.onclose = (e: CloseEvent) => {
            console.log(e.code, e.reason)
        }
    }

    send(message: object): void {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            console.warn("dropping message sent before the socket was open")
            return
        }
        this.ws.send(JSON.stringify(message))
    }

    onMessage(handler: (data: string) => void): void {
        if (!this.ws) {
            return
        }
        this.ws.onmessage = (e: MessageEvent) => handler(e.data)
    }

    onOpen(handler: () => void): void {
        if (!this.ws) {
            return
        }
        this.ws.onopen = () => handler()
    }

    close(): void {
        if (this.ws) {
            // Drop handlers before closing so a teardown doesn't fire onclose
            // logic (e.g. future reconnect) during unmount.
            this.ws.onopen = null
            this.ws.onmessage = null
            this.ws.onerror = null
            this.ws.onclose = null
            this.ws.close()
            this.ws = undefined
        }
    }
}
