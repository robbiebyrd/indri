import type {UpdateMessage} from "@/models/models";

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

    set(data: T, timestamp: Date): void {
        this.setCutoff(timestamp)
        this.baseState = deepClone(data)
        this.deleteBefore(timestamp)
        this.reapply()
    }

    sort(): void {
        this.deltas.sort((a, b) => a.timestamp.getTime() - b.timestamp.getTime())
    }

    update(data: UpdateMessage): void {
        const timestamp = new Date(data.ts)
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
            if (updateMsg.data.removed && updateMsg.data.removed.length > 0) {
                for (const key of updateMsg.data.removed) {
                    state = this.deleteJSONKeyByDotPath(state, key)
                }
            }
            if (updateMsg.data.updated && Object.keys(updateMsg.data.updated).length > 0) {
                for (const [key, value] of Object.entries(updateMsg.data.updated)) {
                    state = this.updateJSONKeyByDotPath(state, key, value)
                }
            }
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
        if (!UNSAFE_KEYS.has(lastPart) && typeof current === 'object' && current !== null && lastPart in current) {
            delete current[lastPart];
        }

        return obj;
    }

}
