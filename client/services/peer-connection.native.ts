import type {PeerConnectionConstructor} from "@indri/protocol-client";

// Loaded on demand: react-native-webrtc touches its native module at import
// time, which fails where that module is missing (Expo Go), and every other
// transport must keep working there. Presence can't be checked up front:
// Expo Go answers NativeModules lookups for modules it lacks with a truthy
// proxy that throws only when used. After a failed load Metro returns
// undefined for the module, so both outcomes are handled.
//
// Not type-checked against PeerConnectionConstructor: its declarations import
// "event-target-shim/index", a path that package's exports map doesn't
// expose, so its addEventListener doesn't resolve under this tsconfig.
export function peerConnection(): PeerConnectionConstructor {
    let ctor: PeerConnectionConstructor | undefined
    let cause = "react-native-webrtc did not load"

    try {
        // eslint-disable-next-line @typescript-eslint/no-require-imports
        ctor = require("react-native-webrtc")?.RTCPeerConnection
    } catch (e) {
        cause = e instanceof Error ? e.message : String(e)
    }

    if (!ctor) {
        throw new Error(
            "The webrtc transport needs a development build (npx expo run:ios / run:android); " +
            `Expo Go doesn't include react-native-webrtc's native module. Cause: ${cause}`,
        )
    }

    return ctor
}
