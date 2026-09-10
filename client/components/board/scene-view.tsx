import {StyleSheet} from "react-native"

import {StyledBox} from "./styled-box"
import {WidgetHost} from "./widget-host"

import type {GridSize} from "@/layout/grid/coords"
import type {SceneLayout} from "@/layout/schema/layout"

export interface SceneViewProps {
    scene: SceneLayout
    /** The board's coordinate space. Scenes do not redefine it. */
    grid: GridSize
}

/**
 * One scene's widgets, over the scene's own background.
 *
 * Every child is keyed by its widget id, never by index. Widget ids are stable
 * across deltas while object identity is not, so an index key would remount a
 * widget whenever an earlier one was added or removed — throwing away its
 * local state and, once Lua lands, its override layer.
 */
export function SceneView({scene, grid}: SceneViewProps) {
    return (
        <StyledBox style={scene.style} boxStyle={styles.scene}>
            {Object.entries(scene.widgets).map(([id, widget]) => (
                <WidgetHost key={id} id={id} widget={widget} grid={grid}/>
            ))}
        </StyledBox>
    )
}

const styles = StyleSheet.create({
    // The containing block for every absolutely-positioned widget below it.
    scene: {
        flex: 1,
        position: 'relative',
    },
})
