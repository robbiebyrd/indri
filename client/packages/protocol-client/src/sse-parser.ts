export type SseEvent = {
    /** The event name; "message" when the stream didn't name one. */
    event: string
    data: string
}

/**
 * Incremental Server-Sent Events parser (the subset of the spec Indri uses:
 * event, data, comments). Push decoded text as it arrives; complete events
 * are passed to onEvent.
 */
export class SseParser {
    private buffer = ""
    private event = ""
    private data: string[] = []
    private readonly onEvent: (e: SseEvent) => void

    constructor(onEvent: (e: SseEvent) => void) {
        this.onEvent = onEvent
    }

    push(text: string) {
        this.buffer += text

        // A trailing "\r" might be the first half of "\r\n"; wait for more.
        let end: number
        while ((end = this.nextLineEnd()) !== -1) {
            const line = this.buffer.slice(0, end)
            const width = this.buffer[end] === "\r" && this.buffer[end + 1] === "\n" ? 2 : 1
            this.buffer = this.buffer.slice(end + width)
            this.line(line)
        }
    }

    private nextLineEnd(): number {
        for (let i = 0; i < this.buffer.length; i++) {
            const c = this.buffer[i]
            if (c === "\n") {
                return i
            }
            if (c === "\r") {
                return i + 1 < this.buffer.length ? i : -1
            }
        }
        return -1
    }

    private line(line: string) {
        if (line === "") {
            if (this.data.length > 0) {
                this.onEvent({event: this.event || "message", data: this.data.join("\n")})
            }
            this.event = ""
            this.data = []
            return
        }

        if (line.startsWith(":")) {
            return
        }

        const colon = line.indexOf(":")
        const field = colon === -1 ? line : line.slice(0, colon)
        let value = colon === -1 ? "" : line.slice(colon + 1)
        if (value.startsWith(" ")) {
            value = value.slice(1)
        }

        if (field === "event") {
            this.event = value
        } else if (field === "data") {
            this.data.push(value)
        }
    }
}
