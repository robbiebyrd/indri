import {useMemo} from "react"
import {StyleSheet, Text, View} from "react-native"

import {parseLayout} from "@/layout/schema/layout"
import {knownWidgetTypes} from "@/layout/registry/registry"
import {SceneView} from "./scene-view"
import {StyledBox} from "./styled-box"

import type {LayoutIssue} from "@/layout/schema/layout"

export interface BoardViewProps {
    /**
     * Raw `game.data.layout`, straight off the wire and completely untrusted.
     * Parsing happens here rather than in the caller so that every board goes
     * through the same normalisation.
     */
    layout: unknown
    /** `stage.currentScene` — which of the layout's scenes to draw. */
    sceneId?: string
}

/**
 * The whole board: parse the layout, draw the current scene over the board's
 * background, and surface anything the parse complained about.
 *
 * `parseLayout` never throws, so there is no error boundary here; every
 * malformed-data path ends in an issue and a message rather than a blank
 * screen.
 */
export function BoardView({layout, sceneId}: BoardViewProps) {
    // Keyed on the raw layout's identity. `GameStateParser` clones the game on
    // every message, so this recomputes more often than it needs to — but the
    // alternative is hashing the layout, which costs more than the parse.
    const {layout: parsed, issues} = useMemo(
        () => parseLayout(layout, {knownWidgetTypes: knownWidgetTypes()}),
        [layout],
    )

    const scene = parsed !== undefined && sceneId !== undefined
        ? parsed.scenes[sceneId]
        : undefined

    return (
        <StyledBox style={parsed?.style} boxStyle={styles.board}>
            {parsed !== undefined && scene !== undefined && (
                <SceneView scene={scene} grid={parsed.grid}/>
            )}
            {parsed === undefined && (
                <BoardMessage text="This board could not be loaded."/>
            )}
            {parsed !== undefined && scene === undefined && (
                <BoardMessage text={missingSceneMessage(sceneId)}/>
            )}
            <IssueOverlay issues={issues}/>
        </StyledBox>
    )
}

function missingSceneMessage(sceneId?: string): string {
    return sceneId === undefined
        ? "No scene is selected."
        : `Scene "${sceneId}" is not in this layout.`
}

/** What a player sees instead of a blank board. Deliberately says nothing technical. */
function BoardMessage({text}: {text: string}) {
    return (
        <View style={styles.message}>
            <Text style={styles.messageText}>{text}</Text>
        </View>
    )
}

/**
 * The parse issue list, DEVELOPMENT ONLY.
 *
 * Players must never see this: the issues name layout paths and internal rules,
 * and a warning-level issue means the board rendered fine anyway. `__DEV__` is
 * a compile-time constant, so the whole overlay is dead-code-eliminated from a
 * production bundle rather than merely hidden.
 *
 * `pointerEvents="none"` because the overlay covers the top of the board and
 * would otherwise steal touches from the widgets underneath it.
 */
function IssueOverlay({issues}: {issues: LayoutIssue[]}) {
    if (!__DEV__ || issues.length === 0) return null

    return (
        <View pointerEvents="none" style={styles.overlay}>
            {issues.map((issue, index) => (
                <Text
                    key={`${issue.severity}:${issue.path}:${index}`}
                    style={issue.severity === 'error' ? styles.issueError : styles.issueWarning}
                >
                    {`${issue.severity.toUpperCase()} ${issue.path || '<root>'} — ${issue.message}`}
                </Text>
            ))}
        </View>
    )
}

const styles = StyleSheet.create({
    // The containing block for the scene, and through it for every widget.
    board: {
        flex: 1,
        position: 'relative',
        overflow: 'hidden',
    },
    message: {
        ...StyleSheet.absoluteFillObject,
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
    },
    messageText: {
        color: '#888',
        fontSize: 16,
        textAlign: 'center',
    },
    overlay: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        maxHeight: '40%',
        overflow: 'hidden',
        padding: 6,
        gap: 2,
        backgroundColor: 'rgba(0, 0, 0, 0.75)',
    },
    issueError: {
        color: '#fca5a5',
        fontSize: 10,
    },
    issueWarning: {
        color: '#fde68a',
        fontSize: 10,
    },
})
