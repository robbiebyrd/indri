package injector

import (
	"net/http"
	"strings"
	"testing"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/transport/ws"
)

func vars(transports string) *envVars.Vars {
	return &envVars.Vars{
		Transports:            transports,
		WSMaxMessageSizeBytes: 32768,
		WSMessageBufferSize:   1024,
		WSPingPeriodSeconds:   54,
		WSPongTimeoutSeconds:  60,
		WebRTCMaxPeers:        4,
	}
}

// routes reports which of the known transport endpoints are mounted.
func routes(t *testing.T, v *envVars.Vars) map[string]bool {
	t.Helper()

	tr, err := newTransport(v)
	if err != nil {
		t.Fatalf("newTransport(%q): %v", v.Transports, err)
	}
	t.Cleanup(func() { _ = tr.Close() })

	mux := http.NewServeMux()
	tr.Register(mux)

	mounted := map[string]bool{}
	for method, path := range map[string]string{
		"GET /ws":            "/ws",
		"GET /sse/stream":    "/sse/stream",
		"GET /graphql":       "/graphql",
		"POST /webrtc/offer": "/webrtc/offer",
	} {
		m := strings.Fields(method)[0]
		req, _ := http.NewRequest(m, "http://x"+path, nil)
		if _, pattern := mux.Handler(req); pattern != "" {
			mounted[path] = true
		}
	}

	return mounted
}

func TestNewTransport_DefaultIsTheBareWebSocketTransport(t *testing.T) {
	tr, err := newTransport(vars("ws"))
	if err != nil {
		t.Fatal(err)
	}

	// One transport is returned as-is, not wrapped in a Composite.
	if _, ok := tr.(*ws.Transport); !ok {
		t.Fatalf("got %T, want *ws.Transport", tr)
	}
}

func TestNewTransport_MountsEachSelectedTransport(t *testing.T) {
	cases := map[string][]string{
		"ws":                      {"/ws"},
		" sse , graphqlws ":       {"/sse/stream", "/graphql"},
		"ws,sse,graphqlws,webrtc": {"/ws", "/sse/stream", "/graphql", "/webrtc/offer"},
	}

	for list, want := range cases {
		t.Run(list, func(t *testing.T) {
			mounted := routes(t, vars(list))

			if len(mounted) != len(want) {
				t.Fatalf("mounted %v, want exactly %v", mounted, want)
			}
			for _, p := range want {
				if !mounted[p] {
					t.Errorf("%v not mounted", p)
				}
			}
		})
	}
}

func TestNewTransport_RejectsBadConfiguration(t *testing.T) {
	cases := map[string]*envVars.Vars{
		"unknown name": vars("ws,carrier-pigeon"),
		// A duplicate would otherwise panic ServeMux on the repeated route.
		"duplicate": vars("ws,sse,ws"),
		"empty":     vars(" , "),
		"udp range reversed": func() *envVars.Vars {
			v := vars("webrtc")
			v.WebRTCUDPPortMin, v.WebRTCUDPPortMax = 50100, 50000
			return v
		}(),
		"udp port out of range": func() *envVars.Vars {
			v := vars("webrtc")
			v.WebRTCUDPPortMin, v.WebRTCUDPPortMax = 50000, 70000
			return v
		}(),
		"no peer cap": func() *envVars.Vars {
			v := vars("webrtc")
			v.WebRTCMaxPeers = 0
			return v
		}(),
	}

	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			if tr, err := newTransport(v); err == nil {
				_ = tr.Close()
				t.Fatalf("newTransport(%q) succeeded, want an error", v.Transports)
			}
		})
	}
}
