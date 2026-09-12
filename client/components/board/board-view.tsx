import {useMemo} from "react"
import {StyleSheet, Text, View} from "react-native"

import {parseLayout} from "@/layout/schema/layout"
import {EMPTY_OVERRIDES, mergeOverrides} from "@/layout/lua/overrides"
import {knownWidgetTypes} from "@/layout/registry/registry"
import {SceneView} from "./scene-view"
import {StyledBox} from "./styled-box"
import {WidgetPressProvider} from "./widget-host"

// Side-effect import: populates the widget registry that `knownWidgetTypes`
// below and `widget-host.tsx` both read. It lives here because this is the
// component that first needs the registry filled, and because a registration
// step in a separate bootstrap file is one that eventually gets forgotten.
import "./widgets"

import type {LayoutIssue} from "@/layout/schema/layout"
import type {Overrides} from "@/layout/lua/overrides"
import type {WidgetPressHandler} from "./widget-host"

export interface BoardViewProps {
    /**
     * Raw `game.data.layout`, straight off the wire and completely untrusted.
     * Parsing happens here rather than in the caller so that every board goes
     * through the same normalisation.
     */
    layout: unknown
    /** `stage.currentScene` — which of the layout's scenes to draw. */
    sceneId?: string
    /**
     * The presentation a script has painted, from `useOverrides`. Composited
     * over the server layout here; the server layout itself is never mutated.
     */
    overrides?: Overrides
    /**
     * Where a press on a widget goes — in practice `LuaHost.emit("widgetPress",
     * …)`. Omitted, the board is inert, which is what a preview wants.
     */
    onWidgetPress?: WidgetPressHandler
}

/**
 * The whole board: parse the layout, composite the script's overrides over it,
 * draw the current scene, and surface anything the parse complained about.
 *
 * THIS IS WHERE LUA MEETS THE RENDERER. A script's `setStyle`/`setConfig` write
 * lands in the override layer and reaches the screen only through the
 * `mergeOverrides` below; without it the whole host API would be inert.
 *
 * `parseLayout` never throws, so there is no error boundary here; every
 * malformed-data path ends in an issue and a message rather than a blank
 * screen.
 */
export function BoardView({layout, sceneId, overrides, onWidgetPress}: BoardViewProps) {
    // Keyed on the raw layout's identity. `GameStateParser` clones the game on
    // every message, so this recomputes more often than it needs to — but the
    // alternative is hashing the layout, which costs more than the parse.
    const {layout: server, issues} = useMemo(
        () => parseLayout(layout, {knownWidgetTypes: knownWidgetTypes()}),
        [layout],
    )

    // Kept separate from the parse so a script painting a cell re-merges
    // without re-validating the whole layout. `mergeOverrides` is
    // structure-sharing, so an untouched subtree comes back by reference and
    // `WidgetHost`'s identity memo still holds.
    const parsed = useMemo(
        () => server === undefined ? undefined : mergeOverrides(server, overrides ?? EMPTY_OVERRIDES),
        [server, overrides],
    )

    const scene = parsed !== undefined && sceneId !== undefined
        ? parsed.scenes[sceneId]
        : undefined

    return (
        <StyledBox style={parsed?.style} boxStyle={styles.board}>
            <WidgetPressProvider value={onWidgetPress}>
                {parsed !== undefined && scene !== undefined && (
                    <SceneView scene={scene} grid={parsed.grid}/>
                )}
            </WidgetPressProvider>
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
