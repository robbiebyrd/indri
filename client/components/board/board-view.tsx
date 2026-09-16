import React, {useCallback, useEffect, useRef, useState} from "react"
import {View, Text, StyleSheet} from "react-native"
import {parseLayout} from "@/layout/schema/layout"
import {StyledBox} from "./styled-box"
import {SceneView} from "./scene-view"
import {LuaSession} from "@/layout/lua/host-api"
import {attachBridge} from "@/layout/lua/bridge"
import {emptyOverrides} from "@/layout/lua/overrides"
import type {OverrideMap} from "@/layout/lua/overrides"
import type {Game} from "@/models/models"
import {lua as luaMod, to_luastring, to_jsstring} from "fengari"

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const luaExt = luaMod as any

/**
 * Install the bare `widget(id)` global in a LuaSession's Lua state.
 *
 * The scene script calls `widget(id).setConfig(patch)` and
 * `widget(id).setStyle(s)`. We wire these to `session.widget(id)` so Lua
 * overrides are stored in the OverrideMap that the React render layer reads.
 */
function installWidgetGlobal(session: LuaSession): void {
    const L = session.L

    luaExt.lua_pushcfunction(L, (innerL: import("fengari").lua_State) => {
        const rawId = luaMod.lua_tostring(innerL, 1)
        const id = rawId ? to_jsstring(rawId) : ""

        luaExt.lua_newtable(innerL)
        const tblIdx = luaMod.lua_gettop(innerL)

        // setConfig(patch) — patch table is arg 1 (dot-notation, no implicit self)
        luaExt.lua_pushcfunction(innerL, (L2: import("fengari").lua_State) => {
            if (luaMod.lua_type(L2, 1) === luaExt.LUA_TTABLE) {
                luaExt.lua_pushstring(L2, to_luastring("text"))
                luaExt.lua_gettable(L2, 1)
                const rawText = luaMod.lua_tostring(L2, -1)
                const text = rawText ? to_jsstring(rawText) : ""
                luaMod.lua_pop(L2, 1)
                session.widget(id).setConfig({text})
            }
            return 0
        })
        luaExt.lua_setfield(innerL, tblIdx, to_luastring("setConfig"))

        // setStyle(s) — style table is arg 1
        luaExt.lua_pushcfunction(innerL, (_L2: import("fengari").lua_State) => {
            // Style overrides from Lua are not yet consumed by the render layer;
            // calling session.widget(id).setStyle() stores them in the OverrideMap
            // and they will be passed as styleOverride to WidgetHost when the
            // render layer is extended to consume them.
            return 0
        })
        luaExt.lua_setfield(innerL, tblIdx, to_luastring("setStyle"))

        return 1
    })
    luaMod.lua_setglobal(L, to_luastring("widget"))
}

interface Props {
    game: Game
    onSend?: (msg: object) => void
}

export function BoardView({game, onSend}: Props) {
    const rawLayout = game.data?.layout
    const {layout, issues} = parseLayout(rawLayout)

    const currentSceneId = game.stage?.currentScene
    const scene = currentSceneId && layout?.scenes[currentSceneId]

    // LuaSession is created once and reused across renders.
    const sessionRef = useRef<LuaSession | null>(null)
    const scriptRunRef = useRef<string | null>(null)

    const [overrides, setOverrides] = useState<OverrideMap>(emptyOverrides)

    // Initialise or reinitialise the session when the send function changes.
    // attachBridge is called once; subsequent onSend changes are ignored.
    // For a game session, onSend is stable (passed from a stable ws ref).
    if (!sessionRef.current) {
        const s = new LuaSession()
        attachBridge(s, (msg) => onSend?.(msg))
        installWidgetGlobal(s)
        sessionRef.current = s
    }
    const session = sessionRef.current

    // Run the scene script once when the scene changes.
    useEffect(() => {
        if (!scene || !scene.script) return
        const scriptKey = currentSceneId + ":" + scene.script
        if (scriptRunRef.current === scriptKey) return
        scriptRunRef.current = scriptKey

        const result = session.runScript(scene.script, `=${currentSceneId}`)
        if (!result.ok) {
            console.warn("[BoardView] scene script error:", result.error)
        }
    }, [scene, currentSceneId, session])

    // Fire stateChanged whenever game changes.
    useEffect(() => {
        if (!currentSceneId) return
        // First call is always a keyframe (overrides cleared)
        session.onStateChange(game, currentSceneId)
        // Mirror overrides out to React state so the render layer updates.
        setOverrides({...session.overrides})
    }, [game, currentSceneId, session])

    const handleWidgetPress = useCallback((widgetId: string) => {
        session.emit("widgetPress", widgetId)
        setOverrides({...session.overrides})
    }, [session])

    return (
        <View style={styles.container}>
            {layout && scene && (
                <StyledBox style={layout.style} viewStyle={styles.board}>
                    <SceneView
                        scene={scene}
                        grid={layout.grid}
                        overrides={overrides}
                        onWidgetPress={handleWidgetPress}
                    />
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
