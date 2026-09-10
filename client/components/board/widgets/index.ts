/**
 * The built-in widget set, and the one place it is registered.
 *
 * Importing this module registers all three widgets as a side effect, because
 * the registry is a process-global map and `getWidget` is called during
 * render: anything that draws a board needs the types to already be there, and
 * a second wiring step somewhere else is a step someone eventually forgets.
 * `components/board/board-view.tsx` is the importer.
 *
 * This module pulls in `.tsx`, so it is NOT loadable by the bare-Node test
 * runner. `widgets.node-test.ts` imports the three `definition.ts` modules
 * instead — the renderer-free half — which is the same data this file
 * registers, plus a `Component`.
 */

import {getWidget, registerWidget} from "@/layout/registry/registry"
import {ImageWidget} from "./image"
import {SubGridWidget} from "./subgrid"
import {TextWidget} from "./text"

import type {WidgetDefinition} from "@/layout/registry/registry"

export {ImageWidget} from "./image"
export {SubGridWidget} from "./subgrid"
export {TextWidget} from "./text"

/** Register every widget the framework ships. Safe to call more than once. */
export function registerBuiltinWidgets(): void {
    registerOnce(TextWidget)
    registerOnce(ImageWidget)
    registerOnce(SubGridWidget)
}

/**
 * Skip a type that is already registered.
 *
 * NOT a way around `registerWidget`'s duplicate check — a conflicting type
 * registered from anywhere else still throws. It exists because Metro's Fast
 * Refresh re-evaluates a module when it or one of its dependencies changes,
 * and an unguarded re-registration would throw during a dev reload and take
 * down the whole board over a cosmetic edit.
 */
function registerOnce<C extends Record<string, unknown>>(def: WidgetDefinition<C>): void {
    if (getWidget(def.type) === undefined) registerWidget(def)
}

registerBuiltinWidgets()
