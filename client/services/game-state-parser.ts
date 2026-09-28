import type {UpdateMessage} from "@/models/models";
import { resolvePath } from "./positional-map.ts"

declare interface Delta {
    timestamp: Date
    data: UpdateMessage
}

// Keys that must never be written through a dot path, to avoid prototype
// pollution from server-supplied paths (which include user/team ids).
const UNSAFE_KEYS = new Set(["__proto__", "constructor", "prototype"])

function deepClone<T>(value: T): T {
    return JSON.parse(JSON.stringify(value))
}

export class GameStateParser<T> {
    private deltas: Delta[] = []
    private cutoff: number = new Date(0).getTime()
    private baseState?: T = undefined
    private currentState?: T = undefined
    private schema?: Record<string, unknown> = undefined
    private layout?: unknown = undefined

    // The schema is copied: positions must stay those of the server's keyframe
    // even if the caller later adds keys (such as the layout) to that object.
    setSchema(schema: Record<string, unknown>): void {
        this.schema = deepClone(schema)
    }

    // The game's layout is sent separately from keyframes and deltas, so it is
    // kept apart and laid over data.layout each time the state is rebuilt: it
    // never enters the positional schema, and a new one keeps the game's state.
    setLayout(layout: unknown): void {
        this.layout = deepClone(layout)
        this.reapply()
    }

    set(data: T, timestamp: Date): void {
        this.setCutoff(timestamp)
        this.baseState = deepClone(data)
        if (!this.schema && data !== null && typeof data === "object" && !Array.isArray(data)) {
            this.setSchema(data as Record<string, unknown>)
        }
        this.deleteBefore(timestamp)
        this.reapply()
    }

    sort(): void {
        this.deltas.sort((a, b) => a.timestamp.getTime() - b.timestamp.getTime())
    }

    update(data: UpdateMessage): void {
        const timestamp = new Date(data.t)
        if (timestamp.getTime() < this.cutoff) {
            return
        }
        this.deltas.push({data, timestamp})
        this.sort()
        this.reapply()
    }

    current(): T | undefined {
        return this.currentState
    }

    private toDotPath(rawPath: unknown): string {
        if (typeof rawPath === "string") {
            // Debug mode: numeric-dotted string like "0.1.1"
            if (this.schema && /^\d/.test(rawPath)) {
                const ints = rawPath.split(".").map(Number)
                return resolvePath(ints, this.schema)
            }
            return rawPath
        }
        if (Array.isArray(rawPath) && this.schema) {
            return resolvePath(rawPath as number[], this.schema)
        }
        // reapply() exits early when baseState is undefined (set() not yet called),
        // so schema is always present when toDotPath runs on number[] paths.
        return String(rawPath)
    }

    private reapply(): void {
        // Deltas can arrive before the first keyframe; hold them until a base
        // state exists rather than applying them onto undefined.
        if (this.baseState === undefined) {
            this.currentState = undefined
            return
        }

        // Always rebuild from a fresh clone so the keyframe snapshot is never
        // mutated and each reapply is deterministic.
        let state: T = deepClone(this.baseState)

        for (const updateMsg of this.deltas) {
            if (updateMsg.data.r) {
                for (const rawPath of updateMsg.data.r) {
                    state = this.deleteJSONKeyByDotPath(state, this.toDotPath(rawPath))
                }
            }
            if (updateMsg.data.u) {
                for (const [rawPath, value] of updateMsg.data.u) {
                    state = this.updateJSONKeyByDotPath(state, this.toDotPath(rawPath), value)
                }
            }
        }

        if (this.layout !== undefined && typeof state === "object" && state !== null) {
            const game = state as Record<string, any>
            if (typeof game.data !== "object" || game.data === null) game.data = {}
            game.data.layout = deepClone(this.layout)
        }

        this.currentState = state
    }

    private setCutoff(date: Date): void {
        this.cutoff = date.getTime()
    }

    private deleteBefore(timestamp: Date): void {
        // Drop deltas already subsumed by the keyframe (at or before its
        // timestamp); keep everything newer so it replays on top.
        const cutoff = timestamp.getTime()
        this.deltas = this.deltas.filter((d) => d.timestamp.getTime() > cutoff)
    }

    private updateJSONKeyByDotPath<S>(obj: S, path: string, value: any): S {
        const parts = path.split('.');
        let current: any = obj;

        for (let i = 0; i < parts.length - 1; i++) {
            const part = parts[i];
            if (UNSAFE_KEYS.has(part)) {
                return obj;
            }
            if (typeof current[part] !== 'object' || current[part] === null) {
                // Preserve arrays when the next segment is a numeric index.
                current[part] = /^\d+$/.test(parts[i + 1]) ? [] : {};
            }
            current = current[part];
        }

        const lastPart = parts[parts.length - 1];
        if (!UNSAFE_KEYS.has(lastPart)) {
            current[lastPart] = value;
        }

        return obj;
    }

    private deleteJSONKeyByDotPath<S>(obj: S, path: string): S {
        const parts = path.split('.');
        let current: any = obj;

        for (let i = 0; i < parts.length - 1; i++) {
            const part = parts[i];
            if (UNSAFE_KEYS.has(part) || typeof current !== 'object' || current === null || !(part in current)) {
                return obj;
            }
            current = current[part];
        }

        const lastPart = parts[parts.length - 1];
        // The server removes only an array's trailing indices, so removing one
        // truncates the array there rather than leaving a hole.
        if (Array.isArray(current) && /^\d+$/.test(lastPart)) {
            current.length = Math.min(current.length, Number(lastPart));
            return obj;
        }
        if (!UNSAFE_KEYS.has(lastPart) && typeof current === 'object' && current !== null && lastPart in current) {
            delete current[lastPart];
        }

        return obj;
    }

}
