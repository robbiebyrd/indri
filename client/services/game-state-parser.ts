import {UpdateMessage} from "@/models/models";

declare interface Delta {
    timestamp: Date
    data: UpdateMessage
}

export class GameStateParser<T> {
    private deltas: Delta[] = []
    private cutoff: number = new Date(0).getTime()
    private baseState?: T = undefined
    private currentState?: T = undefined

    set(data: T, timestamp: Date): void {
        this.setCutoff(timestamp)
        this.baseState = data
        this.currentState = data
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
        if (this.isEmpty()) {
            return
        }

        this.currentState = this.baseState

        for (const updateMsg of this.deltas) {
            if (updateMsg.data.removed && updateMsg.data.removed.length > 0) {
                for (const key of updateMsg.data.removed) {
                    this.currentState = this.deleteJSONKeyByDotPath(this.currentState, key)
                }
            }
            if (updateMsg.data.updated && Object.keys(updateMsg.data.updated).length > 0) {
                for (const [key, value] of Object.entries(updateMsg.data.updated)) {
                    this.currentState = this.updateJSONKeyByDotPath(this.currentState, key, value)
                }
            }
        }
    }

    private setCutoff(date: Date): void {
        this.cutoff = date.getTime()
    }

    private isEmpty(): boolean {
        return this.deltas.length === 0
    }

    private deleteBefore(timestamp: Date): void {
        for (let i = 0; i < this.deltas.length; i++) {
            if (this.deltas[i].timestamp.getTime() > timestamp.getTime()) {
                this.deltas.splice(i, 1)
                i--
            } else {
                break
            }
        }
    }

    private updateJSONKeyByDotPath<T>(obj: T, path: string, value: any): T {
        const parts = path.split('.');
        let current: any = obj;

        for (let i = 0; i < parts.length - 1; i++) {
            const part = parts[i];
            if (typeof current[part] !== 'object' || current[part] === null) {
                current[part] = {};
            }
            current = current[part];
        }

        current[parts[parts.length - 1]] = value;

        return obj;
    }

    private deleteJSONKeyByDotPath<T>(obj: T, path: string): any {
        const parts = path.split('.');
        let current: any = obj;

        for (let i = 0; i < parts.length - 1; i++) {
            const part = parts[i];
            if (typeof current !== 'object' || current === null || !(part in current)) {
                return obj;
            }
            current = current[part];
        }

        const lastPart = parts[parts.length - 1];
        if (typeof current === 'object' && current !== null && lastPart in current) {
            delete current[lastPart];
        }

        return obj;
    }

}