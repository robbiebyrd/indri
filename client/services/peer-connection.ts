import type {PeerConnectionConstructor} from "@indri/protocol-client";

// Browsers provide RTCPeerConnection globally; the .native variant supplies
// react-native-webrtc's on iOS and Android.
export function peerConnection(): PeerConnectionConstructor | undefined {
    return undefined
}
