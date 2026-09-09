// THROWAWAY — story 012 (fengari/Metro spike) only. Delete once the spike is
// signed off; story 019 owns the real Lua runtime.
//
// This screen exists to answer one question that cannot be answered in Node:
// does fengari bundle under Metro and execute under Hermes on a real device?
import {useMemo} from "react"
import {ScrollView, StyleSheet, Text, View} from "react-native"

import {lauxlib, lua, to_jsstring, to_luastring} from "fengari"

import {createSandboxedState} from "@/layout/lua/state"

type Probe = {label: string; expected: string; actual: string; ok: boolean}

function evalLua(src: string): string {
    const L = createSandboxedState()
    if (lauxlib.luaL_loadbuffer(L, to_luastring(src), null, to_luastring("=spike")) !== lua.LUA_OK) {
        return `LOAD ERROR: ${to_jsstring(lua.lua_tostring(L, -1))}`
    }
    if (lua.lua_pcall(L, 0, 1, 0) !== lua.LUA_OK) {
        return `RUNTIME ERROR: ${to_jsstring(lua.lua_tostring(L, -1))}`
    }
    return to_jsstring(lua.lua_tostring(L, -1))
}

function probe(label: string, src: string, expected: string): Probe {
    let actual: string
    try {
        actual = evalLua(src)
    } catch (e) {
        actual = `THREW: ${e instanceof Error ? e.message : String(e)}`
    }
    return {label, expected, actual, ok: actual === expected}
}

export default function LuaSpike() {
    const probes = useMemo<Probe[]>(() => [
        probe("arithmetic", "return 2 + 2", "4"),
        probe("string lib", "return string.rep('ab', 3)", "ababab"),
        probe("table lib", "return table.concat({'a','b'}, '-')", "a-b"),
        probe("math lib", "return math.max(3, 7)", "7"),
        probe("io denied", "return tostring(io)", "nil"),
        probe("os denied", "return tostring(os)", "nil"),
        probe("package denied", "return tostring(package)", "nil"),
        probe("debug denied", "return tostring(debug)", "nil"),
        probe("load denied", "return tostring(load)", "nil"),
        probe("require denied", "return tostring(require)", "nil"),
    ], [])

    const allOk = probes.every((p) => p.ok)

    return (
        <ScrollView contentContainerStyle={styles.container}>
            <Text style={styles.heading}>fengari / Metro spike</Text>
            <Text style={[styles.verdict, allOk ? styles.pass : styles.fail]}>
                {allOk ? "ALL PROBES PASSED" : "SOME PROBES FAILED"}
            </Text>
            {probes.map((p) => (
                <View key={p.label} style={styles.row}>
                    <Text style={styles.label}>{p.label}</Text>
                    <Text style={p.ok ? styles.pass : styles.fail}>
                        {p.ok ? `ok (${p.actual})` : `expected ${p.expected}, got ${p.actual}`}
                    </Text>
                </View>
            ))}
        </ScrollView>
    )
}

const styles = StyleSheet.create({
    container: {padding: 24, gap: 8},
    heading: {fontSize: 22, fontWeight: "600", marginBottom: 4},
    verdict: {fontSize: 16, fontWeight: "700", marginBottom: 12},
    row: {flexDirection: "row", justifyContent: "space-between", gap: 12},
    label: {fontFamily: "monospace"},
    pass: {color: "#15803d"},
    fail: {color: "#b91c1c"},
})
