import {describe, it} from "node:test"
import assert from "node:assert/strict"
import {parseLayout, GameLayoutSchema} from "./layout.ts"

// Full happy-path example layout (from the design doc)
const validLayout = {
    grid: {cols: 12, rows: 12},
    style: {backgroundGradient: {colors: ["#0f172a", "#1e293b"]}},
    script: "-- optional scene script",
    scenes: {
        board: {
            style: {padding: 8},
            script: "-- optional scene script",
            widgets: {
                title: {
                    type: "text",
                    placement: {kind: "grid", col: 0, row: 0, w: 12, h: 2},
                    style: {border: {width: 2, color: "#334155", radius: 8}},
                    config: {text: "Tic Tac Toe", fontSize: 32, align: "center"},
                },
                cells: {
                    type: "subgrid",
                    placement: {kind: "grid", col: 3, row: 3, w: 6, h: 6},
                    config: {
                        grid: {cols: 3, rows: 3},
                        widgets: {
                            c00: {
                                type: "text",
                                placement: {kind: "grid", col: 0, row: 0, w: 1, h: 1},
                                config: {text: ""},
                            },
                        },
                    },
                },
                badge: {
                    type: "image",
                    placement: {kind: "absolute", left: "80%", top: "4%", width: "16%", height: "16%", z: 10},
                    config: {uri: "https://example.com/turn.png", resizeMode: "contain"},
                },
            },
        },
    },
}

describe("parseLayout", () => {
    it("happy path: full example layout parses with zero issues", () => {
        const result = parseLayout(validLayout)
        assert.ok(result.layout !== undefined, "layout should be returned")
        assert.deepEqual(result.issues, [], "should have no issues")
    })

    it("overlapping non-absolute grid widgets → issue, layout still returned", () => {
        const raw = {
            grid: {cols: 12, rows: 12},
            scenes: {
                s1: {
                    widgets: {
                        a: {
                            type: "text",
                            placement: {kind: "grid", col: 0, row: 0, w: 4, h: 4},
                            config: {text: "A"},
                        },
                        b: {
                            type: "text",
                            // Overlaps with widget a
                            placement: {kind: "grid", col: 2, row: 2, w: 4, h: 4},
                            config: {text: "B"},
                        },
                    },
                },
            },
        }
        const result = parseLayout(raw)
        assert.ok(result.layout !== undefined, "layout should still be returned despite overlap")
        assert.ok(result.issues.length > 0, "should have at least one issue for the overlap")
        assert.ok(
            result.issues.some(i => i.message.toLowerCase().includes("overlap")),
            `expected an overlap issue, got: ${JSON.stringify(result.issues)}`
        )
    })

    it("out-of-bounds rect → issue, layout still returned", () => {
        // Widget at col:11, row:11, w:3, h:3 on a 12×12 grid overflows on both axes
        const raw = {
            grid: {cols: 12, rows: 12},
            scenes: {
                s1: {
                    widgets: {
                        oob: {
                            type: "text",
                            placement: {kind: "grid", col: 11, row: 11, w: 3, h: 3},
                            config: {text: "OOB"},
                        },
                    },
                },
            },
        }
        const result = parseLayout(raw)
        assert.ok(result.layout !== undefined, "layout should still be returned")
        assert.ok(result.issues.length > 0, "should have at least one issue for out-of-bounds")
    })

    it("zero-size widget → schema rejection (w: 0 fails)", () => {
        const raw = {
            grid: {cols: 12, rows: 12},
            scenes: {
                s1: {
                    widgets: {
                        bad: {
                            type: "text",
                            placement: {kind: "grid", col: 0, row: 0, w: 0, h: 1},
                            config: {text: "bad"},
                        },
                    },
                },
            },
        }
        const result = parseLayout(raw)
        assert.equal(result.layout, undefined, "layout should be rejected for w:0")
        assert.ok(result.issues.length > 0, "should have schema issues")
    })

    it("unknown widget type → schema rejection", () => {
        const raw = {
            grid: {cols: 12, rows: 12},
            scenes: {
                s1: {
                    widgets: {
                        bad: {
                            type: "video",
                            placement: {kind: "grid", col: 0, row: 0, w: 4, h: 4},
                            config: {uri: "https://example.com/v.mp4"},
                        },
                    },
                },
            },
        }
        const result = parseLayout(raw)
        assert.equal(result.layout, undefined, "layout should be rejected for unknown widget type")
        assert.ok(result.issues.length > 0, "should have schema issues for unknown type")
    })

    it("sub-grid depth cap: subgrid nested 5 levels deep is rejected or flagged", () => {
        // Build 5 levels of nesting by hand
        const inner = {
            type: "text",
            placement: {kind: "grid", col: 0, row: 0, w: 1, h: 1},
            config: {text: "deep"},
        }
        function makeSubgrid(content: unknown, level: number): unknown {
            if (level <= 0) return content
            return {
                type: "subgrid",
                placement: {kind: "grid", col: 0, row: 0, w: 8, h: 8},
                config: {
                    grid: {cols: 8, rows: 8},
                    widgets: {child: makeSubgrid(content, level - 1)},
                },
            }
        }
        // 5 levels of subgrid nesting (exceeds MAX_SUBGRID_DEPTH = 4)
        const raw = {
            grid: {cols: 12, rows: 12},
            scenes: {
                s1: {
                    widgets: {
                        deep: makeSubgrid(inner, 5),
                    },
                },
            },
        }
        const result = parseLayout(raw)
        // Either rejected outright (no layout) or has an issue flagging the depth
        const hasIssue = result.layout === undefined || result.issues.length > 0
        assert.ok(hasIssue, "subgrid at depth 5 should be rejected or flagged with an issue")
    })

    // SanitizeDelta (server-side) strips any path containing a "privateData" segment at any
    // depth, producing silent undebuggable data loss on the client. Accepting a layout that
    // contains a key named "privateData" anywhere would make bugs impossible to diagnose, so
    // any such layout must be rejected outright with an explanatory issue message.
    describe("privateData key anywhere → outright rejection, no layout", () => {
        it("privateData as a widget id", () => {
            const raw = {
                grid: {cols: 12, rows: 12},
                scenes: {
                    s1: {
                        widgets: {
                            privateData: {
                                type: "text",
                                placement: {kind: "grid", col: 0, row: 0, w: 4, h: 4},
                                config: {text: "sneaky"},
                            },
                        },
                    },
                },
            }
            const result = parseLayout(raw)
            assert.equal(result.layout, undefined, "layout must be rejected when privateData is a widget id")
            assert.ok(
                result.issues.some(i => i.message.toLowerCase().includes("privatedata")),
                `expected a privateData issue, got: ${JSON.stringify(result.issues)}`
            )
        })

        it("privateData as a scene key", () => {
            const raw = {
                grid: {cols: 12, rows: 12},
                scenes: {
                    privateData: {
                        widgets: {},
                    },
                },
            }
            const result = parseLayout(raw)
            assert.equal(result.layout, undefined, "layout must be rejected when privateData is a scene key")
            assert.ok(
                result.issues.some(i => i.message.toLowerCase().includes("privatedata")),
                `expected a privateData issue, got: ${JSON.stringify(result.issues)}`
            )
        })

        it("privateData as a key inside widget config", () => {
            const raw = {
                grid: {cols: 12, rows: 12},
                scenes: {
                    s1: {
                        widgets: {
                            w1: {
                                type: "text",
                                placement: {kind: "grid", col: 0, row: 0, w: 4, h: 4},
                                config: {text: "hello", privateData: "secret"},
                            },
                        },
                    },
                },
            }
            const result = parseLayout(raw)
            assert.equal(result.layout, undefined, "layout must be rejected when privateData is inside widget config")
            assert.ok(
                result.issues.some(i => i.message.toLowerCase().includes("privatedata")),
                `expected a privateData issue, got: ${JSON.stringify(result.issues)}`
            )
        })
    })

    it("pixel value in absolute placement rejected (not a percent string)", () => {
        const raw = {
            grid: {cols: 12, rows: 12},
            scenes: {
                s1: {
                    widgets: {
                        abs: {
                            type: "image",
                            placement: {kind: "absolute", left: "10px", top: "0%", width: "10%", height: "10%"},
                            config: {uri: "https://example.com/img.png"},
                        },
                    },
                },
            },
        }
        const result = parseLayout(raw)
        assert.equal(result.layout, undefined, "layout should be rejected for pixel value in absolute placement")
        assert.ok(result.issues.length > 0, "should have schema issues")
    })

    it("cols outside 8..4096 rejected", () => {
        const raw = {
            grid: {cols: 7, rows: 8},
            scenes: {},
        }
        const result = parseLayout(raw)
        assert.equal(result.layout, undefined, "layout should be rejected for cols < 8")
        assert.ok(result.issues.length > 0, "should have schema issues")
    })

    it("parseLayout(null) returns issues, never throws", () => {
        let result: ReturnType<typeof parseLayout> | undefined
        assert.doesNotThrow(() => {
            result = parseLayout(null)
        })
        assert.ok(result !== undefined)
        assert.ok(result!.issues.length > 0, "should have issues for null input")
        assert.equal(result!.layout, undefined)
    })

    it("parseLayout('garbage string') returns issues, never throws", () => {
        let result: ReturnType<typeof parseLayout> | undefined
        assert.doesNotThrow(() => {
            result = parseLayout("garbage string")
        })
        assert.ok(result !== undefined)
        assert.ok(result!.issues.length > 0, "should have issues for non-object input")
        assert.equal(result!.layout, undefined)
    })

    it("absolute widgets may overlap freely with grid widgets — no issue", () => {
        const raw = {
            grid: {cols: 12, rows: 12},
            scenes: {
                s1: {
                    widgets: {
                        grid1: {
                            type: "text",
                            placement: {kind: "grid", col: 0, row: 0, w: 6, h: 6},
                            config: {text: "grid"},
                        },
                        overlay: {
                            type: "image",
                            // Absolute widget "overlapping" the same area as grid1
                            placement: {kind: "absolute", left: "0%", top: "0%", width: "50%", height: "50%"},
                            config: {uri: "https://example.com/overlay.png"},
                        },
                    },
                },
            },
        }
        const result = parseLayout(raw)
        assert.ok(result.layout !== undefined, "layout should be returned")
        const overlapIssues = result.issues.filter(i => i.message.toLowerCase().includes("overlap"))
        assert.equal(overlapIssues.length, 0, "absolute widgets must not produce overlap issues")
    })

    it("GameLayoutSchema direct parse: full example round-trips through schema", () => {
        const parsed = GameLayoutSchema.safeParse(validLayout)
        assert.ok(parsed.success, `schema parse failed: ${!parsed.success ? JSON.stringify(parsed.error) : ""}`)
    })
})
