// The third ClientTransport: inbound over Server-Sent Events (GET /events,
// internal/transport/sse) and outbound over the REST action API
// (POST /api/<action>, internal/transport/rest). See
// plans/client-transport-failover.md, Step 3.
//
// It is structurally unlike the other two. SSE is PUSH-ONLY — the server's own
// package comment says so — so the two directions travel over different
// protocols, and a request/response pair exists here that has no equivalent on
// a WebSocket or a DataChannel: an action's answer comes back in the POST's
// own body, while broadcast deltas arrive on the stream. Both are fed into the
// same onMessage handlers, so MessageHandler cannot tell them apart and does
// not need to.
//
// It is also the only channel that CANNOT be used while logged out. EventSource
// cannot set request headers, so the stream authenticates with a token query
// parameter (sse.go's tokenFrom), and there is no anonymous stream to bind a
// session to later the way `login` over WebSocket does. That is why `connect`
// rejects without a token, and why the supervisor's candidate list is a
// function rather than a fixed array.
import {endpoints} from "./endpoints.ts"
import {Handlers} from "./transport.ts"

import type {ClientTransport} from "./transport.ts"

/** Reads the current session token, or null when there is no session. */
export type TokenSource = () => string | null

/**
 * Fields that are spelled differently over REST than over every other
 * transport. Only `reconnect` differs today (routes.go renames "sessionId" to
 * "token"), and keeping the exceptions in one table is what stops the rename
 * being rediscovered inside `send`.
 */
const FIELD_RENAMES: Record<string, Record<string, string>> = {
    reconnect: {sessionId: "token"},
}

/** A route name is a single path segment — never a path of its own. */
const ROUTE_NAME = /^[a-zA-Z][a-zA-Z0-9_]*$/

/**
 * Turns an outbound message into the REST request that carries it: the action
 * names the route, and everything else is the body.
 *
 * Returns undefined for anything that is not a routable action, rather than
 * POSTing to a URL built from unvalidated input.
 *
 * NOTE: only the BUILT-IN actions have REST routes (rest/routes.go). A
 * game-specific action registered with router.RegisterHandler is WebSocket-only
 * until it is added there too, and over this channel it 404s — which is warned
 * about, not swallowed.
 */
export function restRequest(
    apiBase: string,
    message: object,
): {url: string, body: Record<string, unknown>} | undefined {
    const {action, ...rest} = message as Record<string, unknown>

    if (typeof action !== "string" || !ROUTE_NAME.test(action)) {
        return undefined
    }

    const renames = FIELD_RENAMES[action] ?? {}
    const body: Record<string, unknown> = {}

    for (const [key, value] of Object.entries(rest)) {
        body[renames[key] ?? key] = value
    }

    return {url: `${apiBase}/${action}`, body}
}

export class SseRestTransport implements ClientTransport {
    readonly name = "sse-rest"

    private readonly token: TokenSource

    private stream?: EventSource = undefined
    private apiBase = ""

    private readonly messageHandlers = new Handlers<(data: string) => void>()
    private readonly openHandlers = new Handlers<() => void>()
    private readonly closeHandlers = new Handlers<(reason: string) => void>()

    private pending?: {resolve: () => void, reject: (err: Error) => void} = undefined

    constructor(token: TokenSource) {
        this.token = token
    }

    connect(url: string): Promise<void> {
        const token = this.token()
        if (!token) {
            return Promise.reject(new Error(
                "sse-rest: no session token, so the event stream cannot authenticate",
            ))
        }

        // react-native ships no EventSource. Rejecting keeps this channel out
        // of the rotation on a platform that cannot run it, rather than
        // throwing a ReferenceError up through the supervisor.
        if (typeof EventSource === "undefined") {
            return Promise.reject(new Error("sse-rest: this platform has no EventSource"))
        }

        const routes = endpoints(url)
        this.apiBase = routes.api

        const connected = new Promise<void>((resolve, reject) => {
            this.pending = {resolve, reject}
        })

        // NEVER LOG THIS URL, or anything derived from it: the token is a
        // bearer credential and it travels in the query string because
        // EventSource cannot set an Authorization header.
        const stream = new EventSource(`${routes.events}?token=${encodeURIComponent(token)}`)
        this.stream = stream

        stream.onopen = () => {
            this.settle(true, "")
            this.openHandlers.emit()
        }

        stream.onmessage = (e: MessageEvent) => {
            this.messageHandlers.emit(e.data as string)
        }

        stream.onerror = () => {
            const reason = "sse-rest: event stream error"

            // EventSource reconnects on its own. That is a SECOND reconnection
            // policy competing with the supervisor's, so the stream is closed
            // here and the decision handed upward — one owner, one backoff.
            this.closeStream()

            if (this.settle(false, reason)) {
                return
            }

            this.closeHandlers.emit(reason)
        }

        return connected
    }

    /**
     * Settles the in-flight connect, if there is one. Returns whether it did,
     * which is what tells an error "the stream never opened".
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

    /**
     * Sends an action as a POST. Fire-and-forget, matching every other
     * transport's `send`: the answer comes back through onMessage, so a caller
     * never awaits one.
     */
    send(message: object): void {
        const request = restRequest(this.apiBase, message)
        if (!request) {
            console.warn("sse-rest: dropping a message with no routable action")

            return
        }

        const token = this.token()

        // Over REST the token goes in a header, not the query string: only the
        // stream is forced to use a parameter.
        fetch(request.url, {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                ...(token ? {Authorization: `Bearer ${token}`} : {}),
            },
            body: JSON.stringify(request.body),
        })
            .then(async (response) => {
                const text = await response.text()

                if (!response.ok) {
                    // The action failed. The body is the server's own error
                    // document; it is handed upward like any other response so
                    // an error reaches the app rather than only the console.
                    console.warn(`sse-rest: action failed with status ${response.status}`)
                }

                // An action that only changes state answers with "{}", which
                // MessageHandler ignores: its change arrives as a delta on the
                // stream instead.
                if (text !== "") {
                    this.messageHandlers.emit(text)
                }
            })
            .catch((err: unknown) => {
                // Deliberately not logging the request URL — it is built from
                // the same origin as the token-bearing stream, and a habit of
                // logging URLs here is how the token eventually leaks.
                console.warn("sse-rest: action request failed", err)
            })
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
        this.closeStream()

        // A caller still awaiting connect has to be told, or an abandoned
        // attempt leaves that promise pending forever.
        this.settle(false, "sse-rest: closed before the stream opened")
    }

    /** Tears the stream down without firing onClose — see close()'s contract. */
    private closeStream(): void {
        if (!this.stream) {
            return
        }

        this.stream.onopen = null
        this.stream.onmessage = null
        this.stream.onerror = null
        this.stream.close()
        this.stream = undefined
    }
}
