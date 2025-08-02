export function updateJSONKeyByDotPath<T extends object>(obj: T, path: string, value: any): T {
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

export function deleteJSONKeyByDotPath(obj: any, path: string): any {
    const parts = path.split('.');
    let current = obj;

    for (let i = 0; i < parts.length - 1; i++) {
        const part = parts[i];
        if (typeof current !== 'object' || current === null || !(part in current)) {
            // Path segment does not exist or is not an object
            return obj;
        }
        current = current[part];
    }

    const lastPart = parts[parts.length - 1];
    if (typeof current === 'object' && current !== null && lastPart in current) {
        delete current[lastPart];
        return current;
    }

    return current;
}
