import {base64Decode, Utf8StreamDecoder} from "../codec.ts"
import {SseParser} from "../sse-parser.ts"
import type {SseEvent} from "../sse-parser.ts"
import {BaseTransportClient, resolveFetch} from "../transport.ts"
import type {FetchLike, Payload} from "../transport.ts"

export type SseConfig = {
    /** Base HTTP URL of the server, e.g. http://localhost:5002. */
    url: string
    /**
     * fetch implementation whose response body streams. On React Native pass
     * expo/fetch: the global fetch there buffers the whole body.
     */
    fetch?: FetchLike
}

const CONNECTION_ID_HEADER = "X-Indri-Connection-Id"

/**
 * Server-Sent Events down, POST up. The stream's first event carries the
 * connection ID that every send must present.
 */
export class SseTransportClient extends BaseTransportClient {
    private readonly base: string
    private readonly fetch: FetchLike
    private abort?: AbortController
    private connectionId = ""
    private queue: Promise<void> = Promise.resolve()

    constructor(config: SseConfig) {
        super()
        this.base = config.url.replace(/\/+$/, "")
        this.fetch = resolveFetch(config.fetch, "Pass a streaming fetch implementation in the config.")
    }

    protected start() {
        this.abort = new AbortController()
        void this.stream(this.abort.signal)
    }

    private async stream(signal: AbortSignal) {
        try {
            const res = await this.fetch(`${this.base}/sse/stream`, {
                headers: {Accept: "text/event-stream"},
                signal,
            })

            if (!res.ok || !res.body) {
                this.ended(`event stream refused (${res.status})`)
                return
            }

            const reader = res.body.getReader()
            const decoder = new Utf8StreamDecoder()
            const parser = new SseParser((e) => this.event(e))

            for (;;) {
                const {done, value} = await reader.read()
                if (done) {
                    break
                }
                if (value) {
                    parser.push(decoder.decode(value))
                }
            }

            this.ended("event stream ended")
        } catch (e) {
            if (!signal.aborted) {
                this.ended(e instanceof Error ? e.message : String(e))
            }
        }
    }

    private event(e: SseEvent) {
        switch (e.event) {
            case "connected":
                this.connectionId = e.data
                this.opened()
                break
            case "binary":
                this.emitMessage(base64Decode(e.data))
                break
            case "message":
                this.emitMessage(e.data)
                break
        }
    }

    // Each send waits for the previous one: the server answers only after
    // handling a message, so this keeps them in order.
    protected transmit(data: Payload) {
        this.queue = this.queue.then(() => this.post(data))
    }

    private async post(data: Payload) {
        if (!this.isOpen) {
            return
        }

        try {
            const res = await this.fetch(`${this.base}/sse/send`, {
                method: "POST",
                headers: {
                    [CONNECTION_ID_HEADER]: this.connectionId,
                    "Content-Type": typeof data === "string" ? "application/json" : "application/octet-stream",
                },
                body: data,
            })

            if (res.status === 404) {
                this.ended("the server no longer knows this connection")
            } else if (!res.ok) {
                this.emitError(new Error(`send failed (${res.status})`))
            }
        } catch (e) {
            this.emitError(e)
        }
    }

    protected teardown() {
        this.abort?.abort()
    }
}
