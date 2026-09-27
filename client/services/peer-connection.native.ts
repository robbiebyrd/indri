import type {PeerConnectionConstructor} from "@indri/protocol-client";

// Loaded on demand: react-native-webrtc sets up native event listeners at
// import time, which throws where its native module is missing (Expo Go),
// and every transport but webrtc must keep working there.
//
// Not type-checked against PeerConnectionConstructor: its declarations import
// "event-target-shim/index", a path that package's exports map doesn't
// expose, so its addEventListener doesn't resolve under this tsconfig.
export function peerConnection(): PeerConnectionConstructor {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    return require("react-native-webrtc").RTCPeerConnection
}
