// Package webrtc implements the WebRTC transport: SDP signalling and
// DataChannel-backed connections for clients that prefer peer-to-peer media
// transport over WebSocket/GraphQL.
//
// This package's name collides with github.com/pion/webrtc/v4, which later
// files here import; alias that import as `pion` rather than renaming this
// package.
package webrtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// maxSignalBytes caps a signal body. SDP is attacker-controlled and arrives
// from unauthenticated callers, so an oversized payload is rejected before it
// ever reaches the JSON decoder or pion.
const maxSignalBytes = 256 << 10

// Signal is one step of an SDP exchange. Type is "offer" or "answer"; the
// renegotiation path reuses the same envelope over the signal channel.
type Signal struct {
	Type   string `json:"type"`
	SDP    string `json:"sdp"`
	PeerID string `json:"peerId,omitempty"` // renegotiation only
}

// DecodeSignal reads and validates a Signal from r. It caps the body at
// maxSignalBytes, rejects malformed JSON, and requires a non-empty sdp and a
// type of "offer" or "answer" — all before the caller ever hands the result to
// pion. On any error the returned *Signal is nil, so a caller cannot
// accidentally act on a partially-populated, unvalidated value.
func DecodeSignal(r io.Reader) (*Signal, error) {
	// Read one byte past the cap so a body sized exactly at the limit is
	// distinguishable from one that overflows it, then reject before ever
	// unmarshalling: json.Decoder buffers ahead internally, so checking for
	// leftover bytes after a streaming Decode would be unreliable.
	body, err := io.ReadAll(io.LimitReader(r, maxSignalBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading webrtc signal: %w", err)
	}

	if len(body) > maxSignalBytes {
		return nil, fmt.Errorf("decoding webrtc signal: body exceeds %d bytes", maxSignalBytes)
	}

	var signal Signal

	if err := json.Unmarshal(body, &signal); err != nil {
		if len(body) == 0 {
			return nil, errors.New("decoding webrtc signal: empty body")
		}

		return nil, fmt.Errorf("decoding webrtc signal: %w", err)
	}

	if signal.SDP == "" {
		return nil, errors.New("decoding webrtc signal: missing sdp")
	}

	if signal.Type != "offer" && signal.Type != "answer" {
		return nil, fmt.Errorf("decoding webrtc signal: invalid type %q, want offer or answer", signal.Type)
	}

	return &signal, nil
}
