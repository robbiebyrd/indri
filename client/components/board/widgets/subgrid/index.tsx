import React from "react"
import {View, StyleSheet} from "react-native"
import {getWidget} from "@/layout/registry/registry.ts"
import type {WidgetProps, WidgetContext} from "@/layout/registry/registry.ts"

export type SubgridConfig = {
    grid: {cols: number; rows: number}
    widgets: Record<string, unknown>
}

// Inner widget shape: enough structure to look up and render a nested widget.
type InnerWidget = {
    type: string
    config?: unknown
}

export function SubgridWidget({config, context}: WidgetProps<SubgridConfig>) {
    return (
        // overflow: "hidden" is REQUIRED — RN default is "visible", so absolutely-
        // positioned children would escape the parent bounds without this.
        <View style={styles.container}>
            {Object.entries(config.widgets).map(([id, w]) => {
                const widget = w as InnerWidget
                const def = getWidget(widget.type)
                if (!def) return null

                const innerCtx: WidgetContext = {
                    widgetId: id,
                    sceneId: context.sceneId,
                    override: (patch) => {
                        // Propagate overrides with a prefixed path so the host can
                        // route them to the correct nested widget.
                        context.override({[`widgets.${id}.${Object.keys(patch)[0]}`]: Object.values(patch)[0]})
                    },
                }

                const InnerComponent = def.Component as React.ComponentType<{
                    id: string
                    config: unknown
                    context: WidgetContext
                }>

                return (
                    <InnerComponent
                        key={id}
                        id={id}
                        config={widget.config}
                        context={innerCtx}
                    />
                )
            })}
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
        overflow: "hidden",
    },
})
