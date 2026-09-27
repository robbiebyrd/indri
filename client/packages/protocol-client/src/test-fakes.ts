// Test doubles for the runtime APIs the adapters take by injection.

import type {ChannelLike} from "./transport.ts"

export class FakeWebSocket implements ChannelLike {
    static instances: FakeWebSocket[] = []

    binaryType = "blob"
    readyState = 0
    sent: (string | ArrayBufferLike | ArrayBufferView)[] = []
    closed = false

    onopen: ((ev: unknown) => void) | null = null
    onmessage: ((ev: {data: unknown}) => void) | null = null
    onerror: ((ev: unknown) => void) | null = null
    onclose: ((ev: {code?: number, reason?: string}) => void) | null = null

    readonly url: string
    readonly protocols?: string | string[]

    constructor(url: string, protocols?: string | string[]) {
        this.url = url
        this.protocols = protocols
        FakeWebSocket.instances.push(this)
    }

    static last(): FakeWebSocket {
        return FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
    }

    send(data: string | ArrayBufferLike | ArrayBufferView) {
        this.sent.push(data)
    }

    close() {
        this.closed = true
        this.readyState = 3
    }

    // Server-side actions.
    serverOpen() {
        this.readyState = 1
        this.onopen?.({})
    }

    serverSend(data: unknown) {
        this.onmessage?.({data})
    }

    serverClose(code = 1000, reason = "") {
        this.readyState = 3
        this.onclose?.({code, reason})
    }

    sentJSON(): any[] {
        return this.sent.map((s) => JSON.parse(s as string))
    }
}

/**
 * Wraps a fake fetch so it throws, as browsers do ("Illegal invocation"),
 * when called with any receiver other than the global object.
 */
export function brandCheckedFetch<F extends (...args: any[]) => any>(impl: F): F {
    return function (this: unknown, ...args: any[]) {
        if (this !== undefined && this !== globalThis) {
            throw new TypeError("Failed to execute 'fetch' on 'Window': Illegal invocation")
        }
        return impl(...args)
    } as F
}

/** Collects everything a client emits, for assertions. */
export function record(client: {
    onOpen(cb: () => void): void
    onMessage(cb: (d: string | Uint8Array) => void): void
    onClose(cb: (r: string) => void): void
    onError(cb: (e: unknown) => void): void
}) {
    const log = {opens: 0, messages: [] as (string | Uint8Array)[], closes: [] as string[], errors: [] as unknown[]}
    client.onOpen(() => log.opens++)
    client.onMessage((d) => log.messages.push(d))
    client.onClose((r) => log.closes.push(r))
    client.onError((e) => log.errors.push(e))
    return log
}

/** Captures console.warn for the duration of fn. */
export async function captureWarnings(fn: () => unknown | Promise<unknown>): Promise<string[]> {
    const warnings: string[] = []
    const original = console.warn
    console.warn = (...args: unknown[]) => warnings.push(args.join(" "))
    try {
        await fn()
    } finally {
        console.warn = original
    }
    return warnings
}
