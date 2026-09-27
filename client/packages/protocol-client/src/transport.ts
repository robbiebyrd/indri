/** One message: text (JSON) or opaque bytes (e.g. MessagePack). */
export type Payload = string | Uint8Array

/**
 * A connection to an Indri server over one wire protocol. The client-side
 * counterpart of the server's transport.Conn; every adapter behaves the same.
 */
export interface TransportClient {
    /** Opens the connection. Resolves once messages can be sent; rejects if it can't open. */
    connect(): Promise<void>
    /** Sends one message. Dropped, with a warning, unless the connection is open. */
    send(data: Payload): void
    /** Closes the connection from this side. Does not fire onClose. */
    close(): void
    onOpen(cb: () => void): void
    onMessage(cb: (data: Payload) => void): void
    /** Fires once when the connection ends other than by close(). */
    onClose(cb: (reason: string) => void): void
    onError(cb: (error: unknown) => void): void
}

/** The subset of the WebSocket API the adapters use, so it can be injected. */
export interface WebSocketLike {
    binaryType: string
    readyState: number
    send(data: string | ArrayBufferLike | ArrayBufferView): void
    close(code?: number, reason?: string): void
    onopen: ((ev: any) => void) | null
    onmessage: ((ev: {data: any}) => void) | null
    onerror: ((ev: any) => void) | null
    onclose: ((ev: {code?: number, reason?: string}) => void) | null
}

export type WebSocketConstructor = new (url: string, protocols?: string | string[]) => WebSocketLike

export type FetchLike = (input: string, init?: {
    method?: string
    headers?: Record<string, string>
    body?: string | Uint8Array
    signal?: AbortSignal
}) => Promise<{
    ok: boolean
    status: number
    body: {getReader(): {read(): Promise<{done: boolean, value?: Uint8Array}>}} | null
    json(): Promise<any>
}>

/**
 * Splits a base URL into its root (no trailing slash) and query string, so a
 * query such as ?debug=1 can be kept on the request that opens a connection
 * while paths are appended to the root.
 */
export function splitBaseURL(url: string): {root: string, query: string} {
    const q = url.indexOf("?")
    const root = (q === -1 ? url : url.slice(0, q)).replace(/\/+$/, "")
    return {root, query: q === -1 ? "" : url.slice(q)}
}

/** Returns the injected implementation, else the runtime global, else a clear error. */
export function resolveGlobal<T>(injected: T | undefined, name: string, hint: string): T {
    const found = injected ?? (globalThis as any)[name]
    if (!found) {
        throw new Error(`${name} is not available in this runtime. ${hint}`)
    }
    return found
}

/**
 * Resolves fetch and returns it wrapped so it is always called unbound.
 * Browsers throw "Illegal invocation" when fetch runs with any receiver other
 * than the global object, e.g. when stored on and called through a field.
 */
export function resolveFetch(injected: FetchLike | undefined, hint: string): FetchLike {
    const fetch = resolveGlobal(injected, "fetch", hint)
    return (input, init) => fetch(input, init)
}

/** Detaches every handler before closing, so a close we initiate fires nothing. */
export function closeSocket(ws: WebSocketLike | undefined) {
    if (!ws) {
        return
    }
    ws.onopen = ws.onmessage = ws.onerror = ws.onclose = null
    ws.close()
}

/** Normalizes a received frame: strings stay text, any binary shape becomes a Uint8Array. */
export function toPayload(data: unknown): Payload {
    if (typeof data === "string") {
        return data
    }
    if (data instanceof ArrayBuffer) {
        return new Uint8Array(data)
    }
    if (ArrayBuffer.isView(data)) {
        return new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
    }
    throw new Error("unsupported message type; binary frames must be ArrayBuffers")
}

type State = "idle" | "connecting" | "open" | "closed"

/**
 * The lifecycle every adapter shares. An adapter implements start (begin
 * connecting), transmit, and teardown, and reports progress through opened,
 * ended, emitMessage, and emitError.
 */
export abstract class BaseTransportClient implements TransportClient {
    private state: State = "idle"
    private pending?: {resolve: () => void, reject: (e: unknown) => void}

    private openCb?: () => void
    private messageCb?: (data: Payload) => void
    private closeCb?: (reason: string) => void
    private errorCb?: (error: unknown) => void

    protected abstract start(): void
    protected abstract transmit(data: Payload): void
    protected abstract teardown(): void

    connect(): Promise<void> {
        if (this.state !== "idle") {
            return Promise.reject(new Error("connect() may only be called once per client"))
        }

        this.state = "connecting"

        return new Promise((resolve, reject) => {
            this.pending = {resolve, reject}
            try {
                this.start()
            } catch (e) {
                this.ended(e instanceof Error ? e.message : String(e))
            }
        })
    }

    send(data: Payload) {
        if (this.state !== "open") {
            console.warn("dropping message sent while the connection was not open")
            return
        }
        this.transmit(data)
    }

    close() {
        if (this.state === "closed") {
            return
        }
        const wasConnecting = this.state === "connecting"
        this.state = "closed"
        this.teardown()
        if (wasConnecting) {
            this.pending?.reject(new Error("closed before the connection opened"))
        }
    }

    onOpen(cb: () => void) {
        this.openCb = cb
    }

    onMessage(cb: (data: Payload) => void) {
        this.messageCb = cb
    }

    onClose(cb: (reason: string) => void) {
        this.closeCb = cb
    }

    onError(cb: (error: unknown) => void) {
        this.errorCb = cb
    }

    /** The connection can now send. */
    protected opened() {
        if (this.state !== "connecting") {
            return
        }
        this.state = "open"
        this.pending?.resolve()
        this.openCb?.()
    }

    /** The connection ended from the far side, or failed to open. */
    protected ended(reason: string) {
        if (this.state === "closed") {
            return
        }
        const wasConnecting = this.state === "connecting"
        this.state = "closed"
        this.teardown()
        if (wasConnecting) {
            this.pending?.reject(new Error(reason || "connection failed"))
        } else {
            this.closeCb?.(reason)
        }
    }

    /**
     * Delivers a received message. Also delivered while connecting: a server
     * may send its first message in the same chunk that completes the
     * handshake, before connect() has resolved.
     */
    protected emitMessage(data: Payload) {
        if (this.state !== "closed") {
            this.messageCb?.(data)
        }
    }

    protected emitError(error: unknown) {
        this.errorCb?.(error)
    }

    protected get isOpen(): boolean {
        return this.state === "open"
    }
}
