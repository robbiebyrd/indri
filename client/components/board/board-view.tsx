import React from "react"
import {View, Text, StyleSheet} from "react-native"
import {parseLayout} from "@/layout/schema/layout"
import {StyledBox} from "./styled-box"
import {SceneView} from "./scene-view"
import type {Game} from "@/models/models"

interface Props {
    game: Game
}

export function BoardView({game}: Props) {
    const rawLayout = game.data?.layout
    const {layout, issues} = parseLayout(rawLayout)

    const currentSceneId = game.stage?.currentScene
    const scene = currentSceneId && layout?.scenes[currentSceneId]

    return (
        <View style={styles.container}>
            {layout && scene && (
                <StyledBox style={layout.style} viewStyle={styles.board}>
                    <SceneView scene={scene} grid={layout.grid} />
                </StyledBox>
            )}
            {/* Dev-only issues overlay — never shown to players in production */}
            {__DEV__ && issues.length > 0 && (
                <View style={styles.issuesOverlay}>
                    {issues.map((issue, i) => (
                        <Text key={i} style={styles.issueText}>{issue.path}: {issue.message}</Text>
                    ))}
                </View>
            )}
        </View>
    )
}

const styles = StyleSheet.create({
    container: {flex: 1},
    board: {flex: 1},
    issuesOverlay: {
        position: "absolute",
        bottom: 0,
        left: 0,
        right: 0,
        backgroundColor: "rgba(255,0,0,0.8)",
        padding: 8,
    },
    issueText: {color: "white", fontSize: 12},
})
