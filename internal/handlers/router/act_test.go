package router

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// recorder is a handler that keeps the Request it was handed, so a test can
// assert on what the dispatcher decided to tell it.
type recorder struct {
	requests []actions.Request
}

func (r *recorder) Handle(req actions.Request) (actions.Result, error) {
	r.requests = append(r.requests, req)

	return actions.Result{}, nil
}

// isolate empties the process-global registry for the duration of one test.
func isolate(t *testing.T) {
	t.Helper()

	Reset()
	t.Cleanup(Reset)
}

// TestDispatch_TellsEveryPhaseWhichActionFired is the contract hooks depend on:
// a handler registered under "received" or "processed" runs for every message,
// so the only way it can tell a move from a login is Request.Action. Reporting
// the lifecycle phase there instead would make every hook blind.
func TestDispatch_TellsEveryPhaseWhichActionFired(t *testing.T) {
	isolate(t)

	received, action, processed := &recorder{}, &recorder{}, &recorder{}

	RegisterHandler("hook_received", "received", received)
	RegisterHandler("the_action", "move", action)
	RegisterHandler("hook_processed", "processed", processed)

	if _, err := Dispatch(context.Background(), nil, "move", nil); err != nil {
		t.Fatalf("Dispatch(move) = %v, want no error", err)
	}

	for name, h := range map[string]*recorder{
		"received":  received,
		"move":      action,
		"processed": processed,
	} {
		if len(h.requests) != 1 {
			t.Fatalf("handler registered under %q ran %d times, want 1", name, len(h.requests))
		}

		if got := h.requests[0].Action; got != "move" {
			t.Errorf("handler registered under %q saw action %q, want the dispatched action %q", name, got, "move")
		}
	}
}

// TestDispatch_HandsTheCallersContextToEveryHandler proves the caller's
// deadline reaches the handler. Without it a handler has nothing to pass to a
// game lock or a database call, and a request that has been abandoned still
// holds both.
func TestDispatch_HandsTheCallersContextToEveryHandler(t *testing.T) {
	isolate(t)

	type callerKey struct{}

	ctx := context.WithValue(context.Background(), callerKey{}, "the caller")

	received, action := &recorder{}, &recorder{}

	RegisterHandler("hook_received", "received", received)
	RegisterHandler("the_action", "move", action)

	if _, err := Dispatch(ctx, nil, "move", nil); err != nil {
		t.Fatalf("Dispatch(move) = %v, want no error", err)
	}

	for name, h := range map[string]*recorder{"received": received, "move": action} {
		if len(h.requests) != 1 {
			t.Fatalf("handler registered under %q ran %d times, want 1", name, len(h.requests))
		}

		if h.requests[0].Context != ctx {
			t.Errorf("handler registered under %q got context %v, want the caller's %v",
				name, h.requests[0].Context, ctx)
		}
	}
}

// TestDispatch_PassesTheSessionAndPayloadThrough guards the rest of the Request
// against the two fields added around them.
func TestDispatch_PassesTheSessionAndPayloadThrough(t *testing.T) {
	isolate(t)

	handler := &recorder{}
	RegisterHandler("the_action", "move", handler)

	session := &models.Session{Token: "a-token"}
	payload := map[string]interface{}{"move": "1,1"}

	if _, err := Dispatch(context.Background(), session, "move", payload); err != nil {
		t.Fatalf("Dispatch(move) = %v, want no error", err)
	}

	if handler.requests[0].Session != session {
		t.Errorf("handler got session %v, want %v", handler.requests[0].Session, session)
	}

	if handler.requests[0].Payload["move"] != "1,1" {
		t.Errorf("handler got payload %v, want %v", handler.requests[0].Payload, payload)
	}
}

func TestDispatch_RejectsAnEmptyAction(t *testing.T) {
	isolate(t)

	if _, err := Dispatch(context.Background(), nil, "", nil); err == nil {
		t.Fatal("Dispatch(\"\") = nil, want an error")
	}
}

// TestDispatchMessage_ReadsTheActionFromTheMessage covers the WebSocket entry
// point: the action comes off the wire, and the context comes from the caller.
func TestDispatchMessage_ReadsTheActionFromTheMessage(t *testing.T) {
	isolate(t)

	type callerKey struct{}

	ctx := context.WithValue(context.Background(), callerKey{}, "the caller")

	handler := &recorder{}
	RegisterHandler("the_action", "move", handler)

	msg, err := json.Marshal(map[string]interface{}{"action": "move", "move": "0,2"})
	if err != nil {
		t.Fatalf("marshalling the message: %v", err)
	}

	if _, err := DispatchMessage(ctx, nil, msg); err != nil {
		t.Fatalf("DispatchMessage(move) = %v, want no error", err)
	}

	if len(handler.requests) != 1 {
		t.Fatalf("the move handler ran %d times, want 1", len(handler.requests))
	}

	req := handler.requests[0]

	if req.Action != "move" {
		t.Errorf("handler saw action %q, want %q", req.Action, "move")
	}

	if req.Context != ctx {
		t.Errorf("handler got context %v, want the caller's %v", req.Context, ctx)
	}

	// DecodeMessageWithAction strips "action" from the payload; the handler
	// learns it from Request.Action instead.
	if _, ok := req.Payload["action"]; ok {
		t.Errorf("payload still carries the action key: %v", req.Payload)
	}
}

// TestRequestCtx_FallsBackToBackground keeps a Request built outside the router
// from panicking the first time a handler passes its context to a lock.
func TestRequestCtx_FallsBackToBackground(t *testing.T) {
	if got := (actions.Request{}).Ctx(); got != context.Background() {
		t.Errorf("Request{}.Ctx() = %v, want context.Background()", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if got := (actions.Request{Context: ctx}).Ctx(); got != ctx {
		t.Errorf("Request{Context: ctx}.Ctx() = %v, want the ctx it was given", got)
	}
}
