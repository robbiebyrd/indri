export function parseJsonSafely<T>(jsonString: string): T | undefined {
    try {
        return JSON.parse(jsonString);
    } catch (error) {
        console.error("Failed to parse JSON:", error)
        return undefined
    }
}
