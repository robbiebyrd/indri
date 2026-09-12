package webrtc

import (
	"bytes"
	"strings"
	"testing"
)

func TestDecodeSignal_ValidOffer(t *testing.T) {
	body := `{"type":"offer","sdp":"v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n"}`

	signal, err := DecodeSignal(strings.NewReader(body))
	if err != nil {
		t.Fatalf("DecodeSignal returned error for a valid offer: %v", err)
	}

	if signal == nil {
		t.Fatal("DecodeSignal returned a nil signal for a valid offer")
	}

	if signal.Type != "offer" {
		t.Errorf("Type = %q, want %q", signal.Type, "offer")
	}

	if signal.SDP != "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n" {
		t.Errorf("SDP = %q, want the input sdp preserved", signal.SDP)
	}
}

func TestDecodeSignal_ValidAnswerWithPeerID(t *testing.T) {
	body := `{"type":"answer","sdp":"v=0","peerId":"abc123"}`

	signal, err := DecodeSignal(strings.NewReader(body))
	if err != nil {
		t.Fatalf("DecodeSignal returned error for a valid answer: %v", err)
	}

	if signal.PeerID != "abc123" {
		t.Errorf("PeerID = %q, want %q", signal.PeerID, "abc123")
	}
}

func TestDecodeSignal_MissingSDP(t *testing.T) {
	body := `{"type":"offer"}`

	signal, err := DecodeSignal(strings.NewReader(body))
	if err == nil {
		t.Fatal("DecodeSignal did not reject a payload missing sdp")
	}

	if signal != nil {
		t.Fatal("DecodeSignal returned a non-nil signal on a rejected payload; rejected input must never reach pion")
	}

	if !strings.Contains(err.Error(), "sdp") {
		t.Errorf("error %q does not mention the missing sdp field", err.Error())
	}
}

func TestDecodeSignal_WrongType(t *testing.T) {
	body := `{"type":"candidate","sdp":"v=0"}`

	signal, err := DecodeSignal(strings.NewReader(body))
	if err == nil {
		t.Fatal("DecodeSignal did not reject an unsupported type")
	}

	if signal != nil {
		t.Fatal("DecodeSignal returned a non-nil signal on a rejected payload; rejected input must never reach pion")
	}

	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error %q does not mention the invalid type field", err.Error())
	}
}

func TestDecodeSignal_MissingSDPAndWrongTypeAreDistinctErrors(t *testing.T) {
	_, missingSDPErr := DecodeSignal(strings.NewReader(`{"type":"offer"}`))
	_, wrongTypeErr := DecodeSignal(strings.NewReader(`{"type":"bogus","sdp":"v=0"}`))

	if missingSDPErr == nil || wrongTypeErr == nil {
		t.Fatal("expected both invalid payloads to be rejected")
	}

	if missingSDPErr.Error() == wrongTypeErr.Error() {
		t.Errorf("missing sdp and wrong type produced the same error message %q; they must be distinguishable", missingSDPErr.Error())
	}
}

func TestDecodeSignal_MalformedJSON(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DecodeSignal panicked on malformed JSON: %v", r)
		}
	}()

	signal, err := DecodeSignal(strings.NewReader(`{"type":"offer",`))
	if err == nil {
		t.Fatal("DecodeSignal did not reject malformed JSON")
	}

	if signal != nil {
		t.Fatal("DecodeSignal returned a non-nil signal on malformed JSON; rejected input must never reach pion")
	}
}

func TestDecodeSignal_EmptyBody(t *testing.T) {
	signal, err := DecodeSignal(strings.NewReader(""))
	if err == nil {
		t.Fatal("DecodeSignal did not reject an empty body")
	}

	if signal != nil {
		t.Fatal("DecodeSignal returned a non-nil signal on an empty body; rejected input must never reach pion")
	}
}

func TestDecodeSignal_OversizedBodyRejected(t *testing.T) {
	// One byte over the 256 KiB cap. The oversized filler lives in the sdp
	// field's value so the JSON otherwise stays well-formed.
	huge := bytes.Repeat([]byte("a"), maxSignalBytes+1)
	body := `{"type":"offer","sdp":"` + string(huge) + `"}`

	signal, err := DecodeSignal(strings.NewReader(body))
	if err == nil {
		t.Fatal("DecodeSignal did not reject a body larger than the 256 KiB cap")
	}

	if signal != nil {
		t.Fatal("DecodeSignal returned a non-nil signal on an oversized body; rejected input must never reach pion")
	}
}

func TestDecodeSignal_BodyAtCapIsAccepted(t *testing.T) {
	// Pad the sdp field so the whole body sits at, not over, the cap. The JSON
	// scaffolding around it counts against the cap too, so pad with a small
	// safety margin rather than computing it exactly.
	pad := maxSignalBytes - len(`{"type":"offer","sdp":""}`) - 64
	sdp := bytes.Repeat([]byte("a"), pad)
	body := `{"type":"offer","sdp":"` + string(sdp) + `"}`

	signal, err := DecodeSignal(strings.NewReader(body))
	if err != nil {
		t.Fatalf("DecodeSignal rejected a body within the 256 KiB cap: %v", err)
	}

	if signal == nil {
		t.Fatal("DecodeSignal returned a nil signal for a body within the cap")
	}
}
