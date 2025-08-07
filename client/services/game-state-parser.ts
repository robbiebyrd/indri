declare interface Updates {
    ts: string
    updated: Indexable
    removed: string[]
}

declare interface Item {
    timestamp: Date
    data: Updates
}

declare interface Indexable {
    [key: string]: any
}

export class GameStateParser<Indexable> {
    private updatesList: Item[] = []
    private cutoff: number = new Date(0).getTime()
    private baseState?: Indexable = undefined
    private currentState?: Indexable = undefined

    set(data: Indexable, timestamp: Date): void {
        this.setCutoff(timestamp)
        this.baseState = data
        this.currentState = data
        this.deleteBefore(timestamp)
    }

    sort(): void {
        this.updatesList.sort((a, b) => a.timestamp.getTime() - b.timestamp.getTime())
    }

    update(data: Updates): void {
        const timestamp = new Date(data.ts)
        if (timestamp.getTime() < this.cutoff) {
            return
        }
        this.updatesList.push({data, timestamp})
        this.sort()
        this.reapply()
    }

    current(): Indexable | undefined {
        return this.currentState
    }

    private reapply(): void {
        if (this.isEmpty()) {
            return
        }

        this.currentState = this.baseState

        for (const updateMsg of this.updatesList) {
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
        return this.updatesList.length === 0
    }

    private deleteBefore(timestamp: Date): void {
        for (let i = 0; i < this.updatesList.length; i++) {
            if (this.updatesList[i].timestamp.getTime() > timestamp.getTime()) {
                this.updatesList.splice(i, 1)
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