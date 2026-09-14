// The native event stream. Metro resolves this file in place of
// `sse-stream.ts` on iOS and Android; nothing imports it by name.
//
// react-native ships no EventSource at all, so before this existed the SSE
// channel simply rejected on native and the failover supervisor skipped it,
// leaving phones with two transports instead of three.
//
// react-native-sse is pure JS over XMLHttpRequest — no native module, no Expo
// config plugin, no prebuild. It is NOT used on web (see sse-stream.ts): it
// parses by indexing into `xhr.responseText` and never truncates it, so a
// long-lived stream grows without bound. That cost is acceptable here only
// because this is the LAST-RESORT channel, reached only when WebRTC and
// WebSocket have both failed.
import EventSource from "react-native-sse"

import type {StreamFactory} from "./sse-stream.ts"

export type {EventStream, StreamFactory} from "./sse-stream.ts"

// The library re-opens the stream on this interval after any end or error —
// its own reconnection policy. Zero disables it (`_pollAgain` no-ops when the
// interval is not positive), which is required: the failover supervisor owns
// backoff, and two policies would mean two opinions about which channel is
// live.
const NO_SELF_RECONNECT = 0

export const openEventStream: StreamFactory = (url, token) => {
    // Unlike the browser, this can send a header — so the bearer token stays
    // out of the URL entirely on native. The server prefers the header and
    // only falls back to the query parameter (`sse.go`'s `tokenFrom`).
    const source = new EventSource(url, {
        headers: {Authorization: `Bearer ${token}`},
        pollingInterval: NO_SELF_RECONNECT,
    })

    // Listeners are kept so close() can detach them: this EventSource
    // dispatches a `close` event from close() itself, and more importantly an
    // aborted XHR can still surface an error, which must not be read as a
    // failure of a channel the transport deliberately released.
    let openListener: (() => void) | undefined
    let messageListener: ((e: {data: string | null}) => void) | undefined
    let errorListener: ((e: {type: string}) => void) | undefined

    return {
        onOpen(handler) {
            openListener = () => handler()
            source.addEventListener("open", openListener)
        },
        onMessage(handler) {
            messageListener = (e) => {
                // A comment frame (the server's `:ping` keep-alive) arrives
                // with no data; there is nothing to hand upward.
                if (e.data !== null) {
                    handler(e.data)
                }
            }
            source.addEventListener("message", messageListener)
        },
        onError(handler) {
            // 'error' covers the transport error, the HTTP status error, the
            // request timeout and a thrown exception — every way this library
            // reports that the stream is not usable. The reason is a fixed
            // string: the library's error payload includes `responseText`,
            // and this URL is authenticated.
            errorListener = () => handler("event stream error")
            source.addEventListener("error", errorListener)
        },
        close() {
            if (openListener) source.removeEventListener("open", openListener)
            if (messageListener) source.removeEventListener("message", messageListener)
            if (errorListener) source.removeEventListener("error", errorListener)

            source.close()
        },
    }
}
