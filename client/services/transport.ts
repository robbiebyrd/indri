// Mirrors the server's transport.Transport split (internal/transport) so
// MessageHandler stops hardcoding WebSocket. A second transport (WebRTC, per
// plans/webrtc-transport.md Step 11) plugs in here without MessageHandler or
// the parsers array changing.
//
// `onOpen` sits alongside the four the plan names because MessageHandler's
// own `onOpen`/`send`-before-open contract (see message-handler.node-test.ts)
// depends on knowing when the underlying connection becomes usable, and that
// notion is transport-specific — a WebSocket has `readyState`, WebRTC has its
// own equivalent.
//
// The observer half of this interface is deliberately SUBSCRIPTION-shaped
// rather than setter-shaped (`ws.onmessage = ...`), because FailoverTransport
// (plans/client-transport-failover.md) attaches to a channel, then detaches
// from it when it switches away. A setter can hold exactly one sink, so a
// supervisor sharing a channel with anything else would silently unhook it.

/**
 * Fans one event out to a set of handlers, each with its own unsubscribe.
 *
 * Shared by every ClientTransport implementation so they cannot drift on the
 * two rules that matter: a handler may unsubscribe during its own
 * notification, and a throwing handler must not cost the next one its event.
 */
export class Handlers<T extends (...args: never[]) => void> {
    private readonly handlers = new Set<T>()

    add(handler: T): () => void {
        this.handlers.add(handler)

        return () => {
            this.handlers.delete(handler)
        }
    }

    emit(...args: Parameters<T>): void {
        // Iterate a copy: a handler may unsubscribe itself (or another) while
        // being notified, and mutating the live set mid-iteration would skip
        // the next one.
        for (const handler of [...this.handlers]) {
            try {
                handler(...args)
            } catch (e) {
                // One misbehaving observer must not cost the next its
                // notification — same rule MessageHandler.observe follows.
                console.warn("a transport observer threw:", e)
            }
        }
    }
}

export interface ClientTransport {
    /**
     * Identifies the channel in diagnostics. NEVER branch on it: the whole
     * point of the supervisor is that nothing above this interface knows or
     * cares which channel is live.
     */
    readonly name: string

    /**
     * Opens the channel. Resolves once it is usable, rejects if it fails.
     *
     * The promise is what lets a selection loop tell a working candidate from
     * a dead one. It does NOT bound itself — a channel that neither opens nor
     * errors would leave it pending forever — so a caller that needs a budget
     * wraps it in `connectWithTimeout`, which is the one place that policy
     * lives.
     *
     * A close AFTER a successful open settles nothing: that is a failover
     * signal, delivered through `onClose`, not a connect failure.
     */
    connect(url: string): Promise<void>

    send(message: object): void

    onMessage(handler: (data: string) => void): () => void

    onOpen(handler: () => void): () => void

    /**
     * Reports that an established channel went away, with a human-readable
     * reason for logs. This is the failover trigger.
     *
     * A `close()` by the owner does NOT fire it. Otherwise a supervisor
     * tearing a channel down during a switch would read its own teardown as a
     * failure and immediately switch again.
     */
    onClose(handler: (reason: string) => void): () => void

    close(): void
}

/**
 * Bounds a connect attempt, closing the transport if the budget runs out.
 *
 * Probing costs one attempt per candidate, so an unbounded attempt on a dead
 * channel stalls startup behind a TCP timeout. Closing on expiry matters as
 * much as rejecting: a socket abandoned half-open would otherwise come up
 * later and start delivering messages to a supervisor that has moved on.
 */
export function connectWithTimeout(
    transport: ClientTransport,
    url: string,
    timeoutMs: number,
): Promise<void> {
    return new Promise<void>((resolve, reject) => {
        let settled = false

        const timer = setTimeout(() => {
            if (settled) {
                return
            }
            settled = true
            transport.close()
            reject(new Error(`${transport.name}: connect timed out after ${timeoutMs}ms`))
        }, timeoutMs)

        transport.connect(url).then(
            () => {
                if (settled) {
                    return
                }
                settled = true
                clearTimeout(timer)
                resolve()
            },
            (err: unknown) => {
                if (settled) {
                    return
                }
                settled = true
                clearTimeout(timer)
                reject(err instanceof Error ? err : new Error(String(err)))
            },
        )
    })
}

export class WebSocketTransport implements ClientTransport {
    readonly name = "websocket"

    private ws?: WebSocket = undefined

    // Handlers live on the transport, not on the socket, so they survive a
    // reconnect and can be registered before `connect` is ever called.
    private readonly messageHandlers = new Handlers<(data: string) => void>()
    private readonly openHandlers = new Handlers<() => void>()
    private readonly closeHandlers = new Handlers<(reason: string) => void>()

    // Settles the in-flight connect exactly once. Cleared the moment it
    // settles, which is also how a later close knows the open already
    // happened and is a failover rather than a connect failure.
    private pending?: {resolve: () => void, reject: (err: Error) => void} = undefined

    connect(url: string): Promise<void> {
        const ws = new WebSocket(url)
        this.ws = ws

        const connected = new Promise<void>((resolve, reject) => {
            this.pending = {resolve, reject}
        })

        ws.onopen = () => {
            this.settle(true, "")
            this.openHandlers.emit()
        }

        ws.onmessage = (e: MessageEvent) => {
            this.messageHandlers.emit(e.data as string)
        }

        // A browser reports an error and then a close; only the close carries
        // a code, so the error is left to the close that follows it.
        ws.onerror = () => {
            console.warn("websocket: connection error")
        }

        ws.onclose = (e: CloseEvent) => {
            const reason = `websocket closed (${e.code}${e.reason ? `: ${e.reason}` : ""})`

            // Before open: the candidate never came up, so this is a connect
            // failure. After open: the channel died, which is what a
            // supervisor fails over on.
            if (this.settle(false, reason)) {
                return
            }

            this.closeHandlers.emit(reason)
        }

        return connected
    }

    /**
     * Settles the in-flight connect, if there is one. Returns whether it did,
     * which is what tells a close "the channel never opened".
     */
    private settle(ok: boolean, reason: string): boolean {
        const pending = this.pending
        if (!pending) {
            return false
        }

        this.pending = undefined

        if (ok) {
            pending.resolve()
        } else {
            pending.reject(new Error(reason))
        }

        return true
    }

    send(message: object): void {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            console.warn("dropping message sent before the socket was open")
            return
        }
        this.ws.send(JSON.stringify(message))
    }

    onMessage(handler: (data: string) => void): () => void {
        return this.messageHandlers.add(handler)
    }

    onOpen(handler: () => void): () => void {
        return this.openHandlers.add(handler)
    }

    onClose(handler: (reason: string) => void): () => void {
        return this.closeHandlers.add(handler)
    }

    close(): void {
        if (this.ws) {
            // Drop the socket's own handlers before closing so a teardown
            // cannot reach closeHandlers: a deliberate close is not a
            // failure, and a supervisor must not fail over on its own switch.
            this.ws.onopen = null
            this.ws.onmessage = null
            this.ws.onerror = null
            this.ws.onclose = null
            this.ws.close()
            this.ws = undefined
        }

        // A caller still awaiting connect has to be told, or an abandoned
        // attempt leaves that promise pending forever.
        this.settle(false, "websocket: closed before the connection opened")
    }
}
