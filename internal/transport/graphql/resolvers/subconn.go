package resolvers

import (
	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/graphql/model"
)

// subscriptionBuffer is how many deltas a subscriber may fall behind by before
// its oldest are dropped.
const subscriptionBuffer = 16

// subConn is the push connection behind one gameUpdates subscription. It is the
// shared buffered conn, typed to the channel element gqlgen expects, so the
// broadcast fan-out (which matches on the session key and calls Write) drives
// GraphQL subscribers exactly like WebSocket and SSE clients.
type subConn = transport.BufferedConn[model.JSON]

func newSubConn(sessionID string) *subConn {
	return transport.NewBufferedConn[model.JSON](sessionID, subscriptionBuffer)
}
