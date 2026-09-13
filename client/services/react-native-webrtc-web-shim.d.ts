// react-native-webrtc-web-shim ships no type declarations (no "types" field
// in its package.json, and there is no @types package either). Matching
// layout/lua/fengari.d.ts's precedent for the same problem: a body-form
// `declare module` has to live in a standalone .d.ts, not inside a regular
// .ts module — written there, TypeScript treats the untyped-but-resolvable
// module as already known and rejects the declaration as an invalid
// augmentation (TS2665) instead of accepting it as new.
//
// Only RTCPeerConnection is declared: it is the only export
// webrtc-transport.ts uses. The shim re-exports the browser's own
// RTCPeerConnection on web and react-native-webrtc's implementation on
// native; react-native-webrtc's docs describe that surface as
// W3C-API-compatible, so this borrows the DOM lib's own global
// RTCPeerConnection type (tsconfig's "lib" includes "DOM") rather than `any`.
declare module "react-native-webrtc-web-shim" {
    export const RTCPeerConnection: {
        new (configuration?: RTCConfiguration): RTCPeerConnection
    }
}
