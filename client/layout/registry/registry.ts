import {z} from "zod"
import type {FieldDescriptor} from "./fields.ts"

// WidgetContext: what the renderer passes to a widget's api() function.
export type WidgetContext = {
    widgetId: string
    sceneId: string
    override: (patch: Record<string, unknown>) => void
}

// WidgetProps: what the renderer passes to a widget component.
// React.ComponentType is intentionally absent — this module has zero React Native imports
// and must run in bare Node.
export type WidgetProps<C> = {
    id: string
    config: C
    context: WidgetContext
}

export interface WidgetDefinition<C = unknown> {
    type: string
    schema: z.ZodType<C>          // runtime validation of wire data
    fields: FieldDescriptor[]     // drives the config panel
    defaults: C
    // Component is typed as unknown here — RN components live in client/components/board/widgets/
    Component: unknown
    api(ctx: WidgetContext): Record<string, (...args: unknown[]) => unknown>
}

const registry = new Map<string, WidgetDefinition>()

export function registerWidget<C>(def: WidgetDefinition<C>): void {
    if (registry.has(def.type)) {
        throw new Error(`Widget type "${def.type}" is already registered`)
    }
    registry.set(def.type, def as WidgetDefinition)
}

export function getWidget(type: string): WidgetDefinition | undefined {
    return registry.get(type)
}

export function getAllWidgets(): WidgetDefinition[] {
    return [...registry.values()]
}

// For testing only — reset the registry between tests
export function resetRegistry(): void {
    registry.clear()
}
