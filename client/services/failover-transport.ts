// The failover supervisor (plans/client-transport-failover.md, Step 4).
//
// IT IMPLEMENTS ClientTransport ITSELF. That is the entire design, not a
// convenience: MessageHandler keeps taking exactly one transport and does not
// change, and nothing above this file can tell — or ask — which channel is
// live. Transparency is structural rather than something every caller has to
// cooperate with. The rejected alternative, a transport-aware MessageHandler,
// would have put connection policy next to message parsing and given every
// consumer a notion of "which channel".
//
// Exactly one channel is live at a time. Candidates are probed in preference
// order; the first that comes up wins and is kept until it dies.
import {Handlers, connectWithTimeout} from "./transport.ts"

import type {ClientTransport} from "./transport.ts"

export interface FailoverOptions {
    /**
     * How long one candidate gets to come up. A channel that neither opens nor
     * errors would otherwise stall startup behind a TCP timeout.
     */
    attemptTimeoutMs: number

    /**
     * How long to wait after a cycle in which no candidate came up. Without
     * it, a server outage turns every client into a hot loop across three
     * channels — the client half of a thundering herd.
     */
    backoffMs: number

    /** How many pre-open sends to hold before dropping the oldest. */
    queueLimit: number
}

export class FailoverTransport implements ClientTransport {
    readonly name = "failover"

    /**
     * Re-evaluated on every cycle, never captured once: SSE authenticates at
     * its handshake, so it is not even a legal candidate until a session token
     * exists. A fixed array could not express that.
     */
    private readonly candidates: () => ClientTransport[]
    private readonly opts: FailoverOptions

    private url = ""
    private closed = false
    private running = false

    private active?: ClientTransport = undefined
    private detach: (() => void)[] = []

    /**
     * Sends made before a channel is open. It covers the pre-open window on
     * ONE channel: the moment a candidate is abandoned the queue is dropped,
     * never carried to the next one. The server has no idempotency keys, so a
     * replayed `create` fails and a replayed game action double-applies — a
     * visibly dropped action beats an invisibly duplicated one.
     */
    private queue: object[] = []

    private readonly messageHandlers = new Handlers<(data: string) => void>()
    private readonly openHandlers = new Handlers<() => void>()
    private readonly closeHandlers = new Handlers<(reason: string) => void>()

    private pending?: {resolve: () => void, reject: (err: Error) => void} = undefined
    private backoffTimer?: ReturnType<typeof setTimeout> = undefined
    private wakeBackoff?: () => void = undefined

    // The loop waits here while a channel is healthy. The latch exists
    // because a channel can die BEFORE the loop gets back around to waiting —
    // one that fails the instant it opens, say — and a lost signal with
    // nobody listening would strand the supervisor with no channel and
    // nothing probing.
    private wakeLost?: () => void = undefined
    private lostLatch = false

    constructor(candidates: () => ClientTransport[], opts: FailoverOptions) {
        this.candidates = candidates
        this.opts = opts
    }

    /**
     * Starts supervising. The promise resolves the first time any channel
     * comes up, and rejects only if `close` is called before one does.
     *
     * It deliberately does NOT reject when a cycle fails: the supervisor's job
     * is to keep trying, and a caller that treated a failed cycle as fatal
     * would have to invent its own retry loop — a second one.
     */
    connect(url: string): Promise<void> {
        this.url = url
        this.closed = false

        const connected = new Promise<void>((resolve, reject) => {
            this.pending = {resolve, reject}
        })

        void this.run(0)

        return connected
    }

    send(message: object): void {
        if (this.active) {
            this.active.send(message)

            return
        }

        if (this.queue.length >= this.opts.queueLimit) {
            // Drop-oldest, matching the server's own buffered-connection
            // policy (transport.BufferedConn) rather than inventing a second.
            this.queue.shift()
            console.warn("failover: send queue is full, dropping the oldest message")
        }

        this.queue.push(message)
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
        this.closed = true

        if (this.backoffTimer) {
            clearTimeout(this.backoffTimer)
            this.backoffTimer = undefined
        }

        // Wake a sleeping cycle so it observes `closed` immediately rather
        // than dialling once more when its backoff expires.
        this.wakeBackoff?.()
        this.wakeBackoff = undefined

        // And a loop sitting on a healthy channel, which would otherwise wait
        // for a close that is never coming.
        this.signalLost()

        this.release()
        this.queue = []

        this.settle(false, "failover: closed before any channel came up")
    }

    /**
     * The selection loop. ONE long-lived loop for the whole life of the
     * supervisor: probe a cycle, and either sit on the channel it adopted
     * until that channel dies, or back off and cycle again.
     *
     * Deliberately not "adopt, return, and restart on close". That shape has a
     * re-entrancy race — a channel that dies while the loop is still unwinding
     * asks it to restart before it has stopped, the request is swallowed as
     * "already running", and the supervisor is left with no channel and
     * nothing probing.
     */
    private async run(startAt: number): Promise<void> {
        if (this.running) {
            return
        }

        this.running = true
        let start = startAt

        try {
            while (!this.closed) {
                const list = this.candidates()
                const adopted = list.length > 0 ? await this.probe(list, start % list.length) : -1

                if (adopted >= 0) {
                    await this.untilLost()

                    // Resume AFTER the channel that just died rather than from
                    // the top: a preferred channel that just failed is the
                    // least likely to work, and re-probing it first costs a
                    // full attempt timeout every time.
                    start = adopted + 1

                    continue
                }

                // A fresh cycle always starts from the most preferred channel.
                start = 0

                if (this.closed) {
                    return
                }

                await this.sleep(this.opts.backoffMs)
            }
        } finally {
            this.running = false
        }
    }

    /**
     * Tries each candidate in turn. Returns the index of the one adopted, or
     * -1 if none came up.
     */
    private async probe(list: ClientTransport[], start: number): Promise<number> {
        for (let i = 0; i < list.length; i++) {
            if (this.closed) {
                return -1
            }

            const index = (start + i) % list.length
            const candidate = list[index]

            try {
                await connectWithTimeout(candidate, this.url, this.opts.attemptTimeoutMs)
            } catch (err) {
                console.warn(
                    `failover: ${candidate.name} did not come up:`,
                    err instanceof Error ? err.message : err,
                )
                // Released explicitly: a candidate abandoned half-open would
                // otherwise come up later and deliver frames from a
                // server-side connection nothing is tracking.
                candidate.close()
                this.dropQueue(`${candidate.name} did not come up`)

                continue
            }

            if (this.closed) {
                candidate.close()

                return -1
            }

            this.adopt(candidate)

            return index
        }

        return -1
    }

    private adopt(channel: ClientTransport): void {
        this.active = channel
        this.lostLatch = false

        // onOpen is deliberately NOT subscribed to: `connect` already resolved
        // on open, so subscribing to both would emit twice per connect.
        this.detach = [
            channel.onMessage((data) => this.messageHandlers.emit(data)),
            channel.onClose((reason) => this.lost(channel, reason)),
        ]

        this.settle(true, "")

        // Observers run BEFORE the queue is flushed, which is what lets the
        // app put `reconnect` on the wire ahead of any application message.
        // A connection with no session would reject those anyway.
        this.openHandlers.emit()

        this.flushQueue()
    }

    /** The active channel died. Wake the loop so it switches. */
    private lost(channel: ClientTransport, reason: string): void {
        if (this.active !== channel) {
            // A close from a channel already switched away from. Its
            // server-side connection is gone; nothing here still cares.
            return
        }

        this.release()
        this.dropQueue("the channel changed")

        this.closeHandlers.emit(`failover: ${channel.name} closed (${reason})`)

        this.signalLost()
    }

    /** Resolves when the active channel is lost, or immediately if it already was. */
    private untilLost(): Promise<void> {
        if (this.lostLatch || this.closed) {
            this.lostLatch = false

            return Promise.resolve()
        }

        return new Promise<void>((resolve) => {
            this.wakeLost = resolve
        })
    }

    private signalLost(): void {
        const wake = this.wakeLost

        if (!wake) {
            this.lostLatch = true

            return
        }

        this.wakeLost = undefined
        wake()
    }

    /** Detaches from and closes the active channel, firing nothing. */
    private release(): void {
        for (const off of this.detach) {
            off()
        }
        this.detach = []

        if (this.active) {
            const channel = this.active
            // Cleared first so a re-entrant onClose sees no active channel.
            this.active = undefined
            channel.close()
        }
    }

    private flushQueue(): void {
        const queued = this.queue
        this.queue = []

        for (const message of queued) {
            this.active?.send(message)
        }
    }

    private dropQueue(why: string): void {
        if (this.queue.length === 0) {
            return
        }

        console.warn(
            `failover: dropping ${this.queue.length} queued message(s) because ${why};` +
            " they are not replayed, and the app resyncs instead",
        )
        this.queue = []
    }

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

    /** A backoff that `close` can cut short. */
    private sleep(ms: number): Promise<void> {
        return new Promise<void>((resolve) => {
            this.wakeBackoff = resolve
            this.backoffTimer = setTimeout(resolve, ms)
        })
    }
}
