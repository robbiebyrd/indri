import test from "node:test"
import assert from "node:assert/strict"

import {StyleSchema} from "../schema/style.ts"
import {compileStyle} from "./compile.ts"

// 1. compileStyle(undefined) returns {viewStyle: {}, layers: []} (no crash)
test("compileStyle(undefined) returns empty viewStyle and layers", () => {
    const result = compileStyle(undefined)
    assert.deepEqual(result, {viewStyle: {}, layers: []})
})

// 2. backgroundGradient + backgroundImage both set: layers order is gradient → image
test("layers order: backgroundGradient before backgroundImage", () => {
    const result = compileStyle({
        backgroundGradient: {colors: ["#f00", "#00f"]},
        backgroundImage: {uri: "https://example.com/img.png"},
    })
    assert.equal(result.layers.length, 2)
    assert.equal(result.layers[0].kind, "gradient")
    assert.equal(result.layers[1].kind, "image")
})

// 3. StyleSchema.safeParse with unknown key fails (.strict())
test("StyleSchema rejects unknown keys", () => {
    const result = StyleSchema.safeParse({unknownKey: true})
    assert.equal(result.success, false)
})

// 4. unitless boxShadow rejected
test("StyleSchema rejects unitless boxShadow", () => {
    const result = StyleSchema.safeParse({boxShadow: "2 2 4 #000"})
    assert.equal(result.success, false)
})

// 5. boxShadow with units accepted
test("StyleSchema accepts boxShadow with units", () => {
    const result = StyleSchema.safeParse({boxShadow: "2px 2px 4px rgba(0,0,0,0.5)"})
    assert.equal(result.success, true)
})

// 6. Per-corner radius compiles to per-corner RN props (not borderRadius)
test("per-corner radius compiles to individual RN props", () => {
    const result = compileStyle({
        border: {width: 1, color: "#f00", radius: {topLeft: 4, topRight: 8}},
    })
    assert.equal(result.viewStyle.borderTopLeftRadius, 4)
    assert.equal(result.viewStyle.borderTopRightRadius, 8)
    assert.equal(result.viewStyle.borderRadius, undefined)
    assert.equal(result.viewStyle.borderBottomLeftRadius, undefined)
    assert.equal(result.viewStyle.borderBottomRightRadius, undefined)
})

// 7a. Scalar padding compiles to padding
test("scalar padding compiles to padding", () => {
    const result = compileStyle({padding: 8})
    assert.equal(result.viewStyle.padding, 8)
})

// 7b. Per-edge padding: only defined edges are emitted
test("per-edge padding emits only defined edges", () => {
    const result = compileStyle({padding: {top: 4, bottom: 4}})
    assert.equal(result.viewStyle.paddingTop, 4)
    assert.equal(result.viewStyle.paddingBottom, 4)
    assert.equal(result.viewStyle.paddingLeft, undefined)
    assert.equal(result.viewStyle.paddingRight, undefined)
    assert.equal(result.viewStyle.padding, undefined)
})

// 8. opacity: 0.5 appears in viewStyle
test("compileStyle includes opacity in viewStyle", () => {
    const result = compileStyle({opacity: 0.5})
    assert.equal(result.viewStyle.opacity, 0.5)
})

// 9. No shadow* or elevation in any viewStyle output
test("viewStyle never contains iOS/Android shadow props", () => {
    const result = compileStyle({
        boxShadow: "2px 2px 4px rgba(0,0,0,0.5)",
        backgroundColor: "#fff",
        opacity: 0.8,
    })
    const vs = result.viewStyle as Record<string, unknown>
    assert.equal(vs["shadowColor"], undefined)
    assert.equal(vs["shadowOffset"], undefined)
    assert.equal(vs["shadowOpacity"], undefined)
    assert.equal(vs["shadowRadius"], undefined)
    assert.equal(vs["elevation"], undefined)
})

// 10. Valid style with no unknown keys succeeds
test("StyleSchema accepts a valid style with no unknown keys", () => {
    const result = StyleSchema.safeParse({backgroundColor: "#fff", opacity: 1})
    assert.equal(result.success, true)
    if (result.success) {
        assert.equal(result.data.backgroundColor, "#fff")
        assert.equal(result.data.opacity, 1)
    }
})
