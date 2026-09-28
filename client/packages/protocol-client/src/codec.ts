// Byte codecs implemented here rather than taken from the runtime: React
// Native's Hermes engine does not reliably provide atob/btoa or TextDecoder,
// and the transports must behave the same on web, native, and Node.

const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

const LOOKUP = new Map<string, number>([...ALPHABET].map((c, i) => [c, i]))

/** Standard, padded base64 — the encoding the server uses for binary frames. */
export function base64Encode(bytes: Uint8Array): string {
    let out = ""

    for (let i = 0; i < bytes.length; i += 3) {
        const n = (bytes[i] << 16) | ((bytes[i + 1] ?? 0) << 8) | (bytes[i + 2] ?? 0)

        out += ALPHABET[(n >> 18) & 63] + ALPHABET[(n >> 12) & 63]
        out += i + 1 < bytes.length ? ALPHABET[(n >> 6) & 63] : "="
        out += i + 2 < bytes.length ? ALPHABET[n & 63] : "="
    }

    return out
}

export function base64Decode(text: string): Uint8Array {
    if (text.length % 4 !== 0) {
        throw new Error("invalid base64: length is not a multiple of 4")
    }

    const padding = text.endsWith("==") ? 2 : text.endsWith("=") ? 1 : 0
    const out = new Uint8Array((text.length / 4) * 3 - padding)

    let o = 0
    for (let i = 0; i < text.length; i += 4) {
        let n = 0

        for (let j = 0; j < 4; j++) {
            const c = text[i + j]
            const v = c === "=" && i + j >= text.length - padding ? 0 : LOOKUP.get(c)
            if (v === undefined) {
                throw new Error(`invalid base64 character ${JSON.stringify(c)}`)
            }
            n = (n << 6) | v
        }

        for (const shift of [16, 8, 0]) {
            if (o < out.length) {
                out[o++] = (n >> shift) & 0xff
            }
        }
    }

    return out
}

/**
 * Decodes a UTF-8 byte stream chunk by chunk, carrying a multibyte character
 * that is split across chunks over to the next call. Invalid bytes become
 * U+FFFD.
 */
export class Utf8StreamDecoder {
    private pending: number[] = []

    decode(chunk: Uint8Array): string {
        const bytes = this.pending.length > 0 ? [...this.pending, ...chunk] : chunk
        this.pending = []

        let out = ""
        let i = 0

        while (i < bytes.length) {
            const b = bytes[i]
            const size = b < 0x80 ? 1 : b >> 5 === 0b110 ? 2 : b >> 4 === 0b1110 ? 3 : b >> 3 === 0b11110 ? 4 : 0

            if (size === 0) {
                out += "\uFFFD"
                i++
                continue
            }

            if (i + size > bytes.length) {
                this.pending = Array.from(bytes.slice(i, bytes.length))
                break
            }

            let cp = size === 1 ? b : b & (0xff >> (size + 1))
            let valid = true

            for (let k = 1; k < size; k++) {
                const c = bytes[i + k]
                if (c >> 6 !== 0b10) {
                    valid = false
                    break
                }
                cp = (cp << 6) | (c & 0x3f)
            }

            if (!valid) {
                out += "\uFFFD"
                i++
                continue
            }

            out += String.fromCodePoint(cp)
            i += size
        }

        return out
    }
}
