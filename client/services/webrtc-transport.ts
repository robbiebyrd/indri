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

import {ChunkReassembler, buildOfferSignal, normaliseMessage, parseSignalFromJson, signalUrl, waitForIceGatheringComplete} from "./webrtc-signal.ts"

import type {ClientTransport} from "./transport.ts"

// Channel labels the server dispatches on by exact string match
// (internal/transport/webrtc/webrtc.go gameChannel/signalChannel). A typo
// here is not a type error, so keep these as the one place either is spelled.
const GAME_CHANNEL_LABEL = "game"
const SIGNAL_CHANNEL_LABEL = "signal"

export class WebRTCTransport implements ClientTransport {
    private pc?: RTCPeerConnection = undefined
    private game?: RTCDataChannel = undefined
    // One reassembler per connection: a fresh one every connect(), and
    // dropped in close() so a connection that closes mid chunk-sequence
    // cannot hold its partial buffer anywhere (see ChunkReassembler's
    // comment in webrtc-signal.ts).
    private reassembler?: ChunkReassembler = undefined

    connect(url: string): void {
        const pc = new RTCPeerConnection()
        this.pc = pc
        this.reassembler = new ChunkReassembler()

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

        //TODO: Handle errors appropriately.
        game.onerror = (e: Event) => {
            console.log(e)
        }

        //TODO: Handle reconnects
        game.onclose = () => {
            console.log("webrtc game channel closed")
        }

        this.negotiate(pc, url).catch((err: unknown) => {
            console.log(err)
        })
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

    onMessage(handler: (data: string) => void): void {
        if (!this.game) {
            return
        }
        this.game.onmessage = (e: MessageEvent) => {
            if (!this.reassembler) {
                return
            }
            const message = normaliseMessage(e.data, this.reassembler)
            // undefined means a chunk sequence is still in progress: nothing
            // to hand upward yet (criterion 5).
            if (message !== undefined) {
                handler(message)
            }
        }
    }

    onOpen(handler: () => void): void {
        if (!this.game) {
            return
        }
        this.game.onopen = () => handler()
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
    }
}
