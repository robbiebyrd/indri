package transport

import "log"

// FirstResponse picks the one document a request/response transport answers
// with, and reports every response it therefore cannot deliver.
//
// actions.Result.Responses is a message-stream concept: router.Dispatch merges
// the responses of every handler matching an action, and registration is
// additive, so a dispatch can produce more than one. The stream transports
// deliver all of them — boot.handleClientMessage writes each as its own frame
// to WebSocket and WebRTC clients. A REST request and a GraphQL mutation each
// answer with exactly one document, so they can only deliver the first.
//
// That divergence is deliberate and documented in docs/PROTOCOL.md: aggregating
// into an array would make every caller of every action unwrap a list for a case
// that arises in one action (reconnect into an active game returns an auth
// payload and a keyframe), and a REST/GraphQL caller can recover the dropped
// state from refresh or from its delta stream. What is not acceptable is losing
// responses quietly — a handler whose second response vanishes is
// indistinguishable from one that never produced it — so the drop is logged.
//
// It returns nil when there is nothing to send. "No response" is spelled
// differently per protocol ({} over REST, null over GraphQL), so the caller
// supplies its own empty document.
func FirstResponse(action string, responses [][]byte) []byte {
	if len(responses) == 0 {
		return nil
	}

	if len(responses) > 1 {
		log.Printf(
			"transport: action %q produced %d responses but this transport answers with one document;"+
				" delivering the first and dropping %d (recoverable via refresh or the delta stream)",
			action, len(responses), len(responses)-1,
		)
	}

	return responses[0]
}
