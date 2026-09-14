// The inbound half of the SSE channel, behind an interface, because the two
// platforms need two different implementations and neither can be loaded by
// the other.
//
// THIS FILE IS THE WEB ONE. Metro resolves `sse-stream.native.ts` in its place
// on iOS and Android (its platform-extension resolution, the same mechanism
// behind `.ios.ts`/`.android.ts`). `tsc` and `node --experimental-strip-types`
// both resolve this one, which is what keeps sse-transport.node-test.ts
// loadable — react-native-sse resolves only through Metro.
//
// If that resolution ever silently stops working, native falls back to this
// file, `EventSource` is undefined there, and `connect` rejects — so the
// channel drops out of the failover rotation exactly as it did before
// react-native-sse was added. A degradation, not a crash.

/**
 * The slice of an event stream the transport needs. Handlers are single
 * setters rather than subscriptions: exactly one owner (SseRestTransport)
 * consumes a stream, and it creates a new one per connect.
 */
export interface EventStream {
    onOpen(handler: () => void): void
    onMessage(handler: (data: string) => void): void

    /**
     * Reports that the stream failed or ended. The implementation must NOT
     * reconnect on its own — the failover supervisor owns that policy, and a
     * second one competing with it means two backoffs and two opinions about
     * which channel is live.
     */
    onError(handler: (reason: string) => void): void

    /** Stops the stream. Must not invoke onError — a teardown is not a failure. */
    close(): void
}

/**
 * Opens a stream. `token` is passed separately from `url` on purpose: web has
 * to append it as a query parameter because the browser's EventSource cannot
 * set headers, while native sends it as `Authorization: Bearer`. The server
 * accepts either and prefers the header (`sse.go`'s `tokenFrom`).
 */
export type StreamFactory = (url: string, token: string) => EventStream

/**
 * The browser's own EventSource. Deliberately NOT replaced by react-native-sse
 * on web: that library parses by indexing into `xhr.responseText` and never
 * truncates it, so a stream held open for a whole game session grows without
 * bound. EventSource has no such cost.
 */
export const openEventStream: StreamFactory = (url, token) => {
    if (typeof EventSource === "undefined") {
        throw new Error("sse-rest: this platform provides no event stream")
    }

    // NEVER LOG THIS URL, or anything derived from it: the token is a bearer
    // credential and it is in the query string only because EventSource cannot
    // send a header.
    const source = new EventSource(`${url}?token=${encodeURIComponent(token)}`)

    return {
        onOpen(handler) {
            source.onopen = () => handler()
        },
        onMessage(handler) {
            source.onmessage = (e: MessageEvent) => handler(e.data as string)
        },
        onError(handler) {
            source.onerror = () => handler("event stream error")
        },
        close() {
            // Handlers first: EventSource fires onerror as part of closing, and
            // the transport must not read its own teardown as a failure.
            source.onopen = null
            source.onmessage = null
            source.onerror = null
            source.close()
        },
    }
}
