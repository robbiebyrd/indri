package injector

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/transport"
	"github.com/robbiebyrd/indri/internal/transport/graphqlws"
	"github.com/robbiebyrd/indri/internal/transport/sse"
	"github.com/robbiebyrd/indri/internal/transport/webrtc"
	"github.com/robbiebyrd/indri/internal/transport/ws"
)

const (
	// webrtcGatherTimeout must stay under the HTTP server's 10s WriteTimeout
	// (entrypoints/http.go), since the offer response waits on gathering.
	webrtcGatherTimeout = 5 * time.Second
	// webrtcOpenTimeout leaves headroom over observed setup times: ~2s on a
	// host offering many interfaces.
	webrtcOpenTimeout = 15 * time.Second
	// graphqlInitTimeout is how long a graphqlws client has to send
	// connection_init.
	graphqlInitTimeout = 10 * time.Second
)

// newTransport builds the transports named in vars.Transports. A single
// transport is returned as-is; several run concurrently behind a Composite.
func newTransport(vars *envVars.Vars) (transport.Transport, error) {
	var (
		built []transport.Transport
		seen  = map[string]bool{}
	)

	for _, name := range strings.Split(vars.Transports, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		if seen[name] {
			return nil, fmt.Errorf("transport %q listed twice in INDRI_TRANSPORTS", name)
		}
		seen[name] = true

		t, err := buildTransport(name, vars)
		if err != nil {
			return nil, err
		}

		built = append(built, t)
	}

	switch len(built) {
	case 0:
		return nil, errors.New("INDRI_TRANSPORTS names no transports")
	case 1:
		return built[0], nil
	default:
		return transport.Composite(built...), nil
	}
}

func buildTransport(name string, vars *envVars.Vars) (transport.Transport, error) {
	switch name {
	case "ws":
		return ws.New(), nil
	case "sse":
		return sse.New(sse.Config{
			AllowedOrigins: vars.AllowedOrigins,
			BufferSize:     vars.WSMessageBufferSize,
			MaxMessageSize: int64(vars.WSMaxMessageSizeBytes),
			PingPeriod:     time.Duration(vars.WSPingPeriodSeconds) * time.Second,
		}), nil
	case "graphqlws":
		return graphqlws.New(graphqlws.Config{
			AllowedOrigins: vars.AllowedOrigins,
			BufferSize:     vars.WSMessageBufferSize,
			MaxMessageSize: int64(vars.WSMaxMessageSizeBytes),
			PingPeriod:     time.Duration(vars.WSPingPeriodSeconds) * time.Second,
			PongWait:       time.Duration(vars.WSPongTimeoutSeconds) * time.Second,
			InitTimeout:    graphqlInitTimeout,
		}), nil
	case "webrtc":
		return buildWebRTC(vars)
	default:
		return nil, fmt.Errorf("unknown transport %q in INDRI_TRANSPORTS (want ws, sse, graphqlws, or webrtc)", name)
	}
}

func buildWebRTC(vars *envVars.Vars) (transport.Transport, error) {
	if vars.WebRTCMaxPeers <= 0 {
		return nil, errors.New("INDRI_WEBRTC_MAX_PEERS must be positive")
	}

	for _, p := range []int{vars.WebRTCUDPPortMin, vars.WebRTCUDPPortMax} {
		if p < 0 || p > math.MaxUint16 {
			return nil, fmt.Errorf("webrtc udp port %d out of range", p)
		}
	}

	t, err := webrtc.New(webrtc.Config{
		AllowedOrigins: vars.AllowedOrigins,
		BufferSize:     vars.WSMessageBufferSize,
		ICEServers:     splitList(vars.WebRTCICEServers),
		MaxPeers:       vars.WebRTCMaxPeers,
		NAT1To1IPs:     splitList(vars.WebRTCNAT1To1IPs),
		UDPPortMin:     uint16(vars.WebRTCUDPPortMin),
		UDPPortMax:     uint16(vars.WebRTCUDPPortMax),
		GatherTimeout:  webrtcGatherTimeout,
		OpenTimeout:    webrtcOpenTimeout,
	})
	if err != nil {
		return nil, err
	}

	return t, nil
}

func splitList(s string) []string {
	var out []string

	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}

	return out
}
