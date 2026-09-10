import {StyleSheet, View} from "react-native"

import {SubGridConfigSchema} from "@/layout/schema/widget"
import {SceneView} from "../../scene-view"
import {WidgetConfigError} from "../config-error"
import {SUBGRID_TYPE, subGridDefinition} from "./definition"

import type {WidgetDefinition, WidgetRenderProps} from "@/layout/registry/registry"
import type {SubGridWidgetConfig} from "./definition"

/** RN's default is `visible`; a sub-grid clips unless the author says otherwise. */
const DEFAULT_OVERFLOW = "hidden"

/**
 * A nested coordinate space with its own widgets.
 *
 * Placement is NOT reimplemented here. `SceneView` already draws a widget map
 * over a grid and `WidgetHost` already turns a placement into a percentage
 * box, so a sub-grid is those two components pointed at a smaller grid. That
 * is also what makes the recursion work at all: a sub-grid inside a sub-grid
 * is just this component again, reached through the same registry lookup.
 *
 * DEPTH IS NOT CAPPED HERE. `parseLayout` empties the children of anything
 * past `MAX_SUBGRID_DEPTH` before this ever renders, so a second cap would be
 * a duplicate rule that could disagree with the first. This draws what it is
 * given.
 */
export function SubGridWidgetView({id, widget}: WidgetRenderProps) {
    const parsed = SubGridConfigSchema.safeParse(widget.config ?? {})
    if (!parsed.success) {
        return <WidgetConfigError id={id} type={SUBGRID_TYPE} error={parsed.error}/>
    }

    const {grid, widgets} = parsed.data

    // Clipping is the sub-grid's whole structural job. Children are absolutely
    // positioned in percentages of THIS box, and RN's default `overflow:
    // visible` would let a child whose rect was authored past the edge — or an
    // absolute child at "120%" — draw over the rest of the board with nothing
    // to show which widget it escaped from. The authored style still wins, so
    // an author who wants overspill can ask for it.
    const overflow = widget.style?.overflow ?? DEFAULT_OVERFLOW

    return (
        <View style={[styles.container, {overflow}]}>
            <SceneView scene={{widgets}} grid={grid}/>
        </View>
    )
}

/** The full definition: the config contract plus its renderer. */
export const SubGridWidget: WidgetDefinition<SubGridWidgetConfig> = {
    ...subGridDefinition,
    Component: SubGridWidgetView,
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
    },
})
