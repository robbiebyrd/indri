// WebRTC ClientTransport: reaches the server's signalling route
// (POST /rtc/offer, internal/transport/webrtc/webrtc.go) over a DataChannel
// instead of a WebSocket. See plans/webrtc-transport.md, Step 11, and the
// "Client" bullet under Research Findings.
//
// The value import is from react-native-webrtc-web-shim, not
// react-native-webrtc directly and not a bare global: the shim re-exports the
// browser's own RTCPeerConnection on web and react-native-webrtc's
// implementation on native, so this file is the ONE code path that serves
// both (criterion 4). Its types come from ./react-native-webrtc-web-shim.d.ts
// — the shim itself ships none.
//
// This is also why the pure parts (the signal envelope, URL derivation,
// payload normalisation, the gather-complete wait) live in webrtc-signal.ts
// rather than here: the shim's own internals use extensionless relative
// imports Node's ESM loader cannot resolve, so importing anything at all from
// THIS file crashes under `node --experimental-strip-types`. Keeping the
// testable logic in webrtc-signal.ts is what lets
// webrtc-transport.node-test.ts run at all.
import {RTCPeerConnection} from "react-native-webrtc-web-shim"

import {Handlers} from "./transport.ts"
import {ChunkReassembler, buildOfferSignal, normaliseMessage, parseSignalFromJson, signalUrl, waitForIceGatheringComplete} from "./webrtc-signal.ts"

import type {ClientTransport} from "./transport.ts"

// Channel labels the server dispatches on by exact string match
// (internal/transport/webrtc/webrtc.go gameChannel/signalChannel). A typo
// here is not a type error, so keep these as the one place either is spelled.
const GAME_CHANNEL_LABEL = "game"
const SIGNAL_CHANNEL_LABEL = "signal"

export class WebRTCTransport implements ClientTransport {
    readonly name = "webrtc"

    private pc?: RTCPeerConnection = undefined
    private game?: RTCDataChannel = undefined
    // One reassembler per connection: a fresh one every connect(), and
    // dropped in close() so a connection that closes mid chunk-sequence
    // cannot hold its partial buffer anywhere (see ChunkReassembler's
    // comment in webrtc-signal.ts).
    private reassembler?: ChunkReassembler = undefined

    private readonly messageHandlers = new Handlers<(data: string) => void>()
    private readonly openHandlers = new Handlers<() => void>()
    private readonly closeHandlers = new Handlers<(reason: string) => void>()

    private pending?: {resolve: () => void, reject: (err: Error) => void} = undefined

    connect(url: string): Promise<void> {
        const pc = new RTCPeerConnection()
        this.pc = pc
        this.reassembler = new ChunkReassembler()

        const connected = new Promise<void>((resolve, reject) => {
            this.pending = {resolve, reject}
        })

        // "game" carries every action to/from the server, and is left at the
        // DataChannel default — reliable, ordered (no maxRetransmits or
        // maxPacketLifeTime, ordered untouched) — because that is what makes
        // it match Indri's existing delta-delivery contract. A dropped or
        // reordered delta has no game-level resend to fall back on, so the
        // transport itself has to guarantee both.
        const game = pc.createDataChannel(GAME_CHANNEL_LABEL)
        // Never Blob (the browser default): normaliseMessage only handles
        // string and ArrayBuffer, and Blob.text() is async where onmessage
        // must hand back a value synchronously.
        game.binaryType = "arraybuffer"
        this.game = game

        // "signal" exists only so a future renegotiation (server story 041)
        // has a channel to send its offers on. This transport never sends or
        // reads anything on it.
        pc.createDataChannel(SIGNAL_CHANNEL_LABEL)

        game.onopen = () => {
            this.settle(true, "")
            this.openHandlers.emit()
        }

        game.onmessage = (e: MessageEvent) => {
            if (!this.reassembler) {
                return
            }
            const message = normaliseMessage(e.data, this.reassembler)
            // undefined means a chunk sequence is still in progress: nothing
            // to hand upward yet (criterion 5).
            if (message !== undefined) {
                this.messageHandlers.emit(message)
            }
        }

        game.onerror = () => {
            console.warn("webrtc: data channel error")
        }

        game.onclose = () => {
            const reason = "webrtc data channel closed"

            // Before open the candidate never came up, which is a connect
            // failure; after open it is a live channel dying, which is what a
            // supervisor fails over on.
            if (this.settle(false, reason)) {
                return
            }

            this.closeHandlers.emit(reason)
        }

        // Signalling failing is a connect failure like any other: reject so a
        // supervisor moves to the next candidate instead of waiting on a
        // channel that will never open.
        this.negotiate(pc, url).catch((err: unknown) => {
            this.settle(false, err instanceof Error ? err.message : String(err))
        })

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

    private async negotiate(pc: RTCPeerConnection, url: string): Promise<void> {
        await pc.setLocalDescription(await pc.createOffer())

        // THE SERVER IS NON-TRICKLE: gather fully before the offer is ever
        // sent, or the answer's candidates would be incomplete with no way
        // to fill them in later.
        await waitForIceGatheringComplete(pc)

        const local = pc.localDescription
        if (!local) {
            throw new Error("webrtc: no local description after ICE gathering completed")
        }

        const response = await fetch(signalUrl(url), {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify(buildOfferSignal(local)),
        })
        if (!response.ok) {
            throw new Error(`webrtc: signalling failed with status ${response.status}`)
        }

        const answer = parseSignalFromJson(await response.text())
        if (!answer) {
            throw new Error("webrtc: signalling returned a malformed answer")
        }

        await pc.setRemoteDescription({type: answer.type as RTCSdpType, sdp: answer.sdp})
    }

    send(message: object): void {
        if (!this.game || this.game.readyState !== "open") {
            console.warn("dropping message sent before the data channel was open")
            return
        }
        this.game.send(JSON.stringify(message))
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
        // Drop the reassembler before anything else: criterion 3, a
        // connection closing mid-sequence must not hold its partial buffer.
        this.reassembler = undefined

        if (this.game) {
            // Drop handlers before closing, matching WebSocketTransport: a
            // teardown must not fire onclose logic (e.g. a future reconnect)
            // during unmount.
            this.game.onopen = null
            this.game.onmessage = null
            this.game.onerror = null
            this.game.onclose = null
            this.game.close()
            this.game = undefined
        }
        if (this.pc) {
            this.pc.close()
            this.pc = undefined
        }

        // A caller still awaiting connect has to be told, or an abandoned
        // attempt leaves that promise pending forever.
        this.settle(false, "webrtc: closed before the data channel opened")
    }
}
