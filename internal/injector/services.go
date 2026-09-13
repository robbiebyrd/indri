package injector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/router"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	authSevice "github.com/robbiebyrd/indri/internal/services/authentication"
	broadcastService "github.com/robbiebyrd/indri/internal/services/broadcast"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
	luaService "github.com/robbiebyrd/indri/internal/services/lua"
	schedulerService "github.com/robbiebyrd/indri/internal/services/scheduler"
	sessionService "github.com/robbiebyrd/indri/internal/services/session"
	userService "github.com/robbiebyrd/indri/internal/services/user"
	"github.com/robbiebyrd/indri/internal/transport"
	graphqlTransport "github.com/robbiebyrd/indri/internal/transport/graphql"
	restTransport "github.com/robbiebyrd/indri/internal/transport/rest"
	sseTransport "github.com/robbiebyrd/indri/internal/transport/sse"
	webrtcTransport "github.com/robbiebyrd/indri/internal/transport/webrtc"
)

func GetServices(ctx context.Context, clients *ClientsInjector, repos *ReposInjector) (*ServicesInjector, error) {
	if clients == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	if repos == nil {
		return nil, errors.New("clients were not passed to the repo injector")
	}

	// These transports need the session store (to authenticate bearer tokens),
	// which only exists now, so they are built here and aggregated with the
	// WebSocket transport from GetClients. Everything downstream targets the
	// aggregate.
	//
	// sse is push-only and rest is request-only: together they are the third
	// way to play, alongside WebSocket and GraphQL. webrtc is bidirectional
	// over a DataChannel, shaped like ws, and is the fourth.
	gql := graphqlTransport.New(repos.SessionRepo)
	events := sseTransport.New(repos.SessionRepo)
	api := restTransport.New(repos.SessionRepo)

	vars := envVars.GetEnv()

	// Unlike the other three, New builds a pion API and a shared UDP mux, so
	// it can fail (e.g. the configured port is unavailable) and must be
	// handled rather than panicked through.
	rtc, err := webrtcTransport.New(repos.SessionRepo, webrtcTransport.Config{
		UDPPort:    vars.RTCUDPPort,
		NAT1To1IPs: rtcNAT1To1IPs(vars.RTCNAT1To1IPs),
	})
	if err != nil {
		return nil, err
	}

	rtc.MaxPeers = vars.RTCMaxPeers
	rtc.PendingTTL = time.Duration(vars.RTCPendingTTLSeconds) * time.Second

	multi := transport.NewMulti(clients.Transport, gql, events, api, rtc)

	// Each needs the aggregate so a kick closes the target's connections on
	// every transport, not just its own.
	for _, t := range []interface{ SetPeer(transport.Transport) }{gql, events, api, rtc} {
		t.SetPeer(multi)
	}

	clients.Transport = multi

	gs, err := gameService.NewService(repos.GameRepo, repos.ScriptRepo)
	if err != nil {
		return nil, err
	}

	bs, err := broadcastService.NewService(ctx, clients.Transport, repos.UserRepo, repos.SessionRepo)
	if err != nil {
		return nil, err
	}

	as, err := authSevice.NewService(repos.UserRepo, repos.SessionRepo)
	if err != nil {
		return nil, err
	}

	us, err := userService.NewService(repos.UserRepo)
	if err != nil {
		return nil, err
	}

	ss := sessionService.NewService(repos.SessionRepo)

	// Built last, and nothing may fail after it: compiling the scripts also
	// builds the first pooled Lua state, and an error returned past this point
	// would drop the engine without closing it.
	le, err := luaService.NewEngineWithGrants(repos.ScriptRepo.Get().Scripts, repos.GameRepo)
	if err != nil {
		return nil, fmt.Errorf("loading the game scripts: %w", err)
	}

	// Set here rather than inside the engine, so the lua package never imports
	// the handler layer. Without it indri.send refuses, which would make a
	// script's events silently stop working on a server that loaded fine.
	le.Dispatch = router.Dispatch

	// Both halves of the timer feature, wired to the same store. The assignment
	// is also the compile-time check that the adapter still satisfies what the
	// lua package asks for.
	timers, err := schedulerService.NewTimers(repos.ScheduleRepo)
	if err != nil {
		return nil, closeEngine(le, err)
	}

	le.Timers = timers

	// Built from the engine's manifest, which is frozen by now: a timer may only
	// fire an action a game script registered, because every built-in refuses the
	// nil session a timer fires with.
	sched, err := schedulerService.New(repos.ScheduleRepo, router.Dispatch, le.Actions(), schedulerService.Config{})
	if err != nil {
		return nil, closeEngine(le, err)
	}

	return &ServicesInjector{
		GameService:      gs,
		BroadcastService: bs,
		AuthService:      as,
		UserService:      us,
		SessionService:   ss,
		LuaEngine:        le,
		Scheduler:        sched,
	}, nil
}

// closeEngine releases the engine on a failure after it was built.
//
// Nothing used to be allowed to fail past NewEngineWithGrants, because an error
// returned there would drop every pooled Lua state without closing it. Two
// things now do have to happen afterwards — both need the engine's manifest —
// so the rule becomes "release it on the way out" instead.
func closeEngine(le *luaService.Engine, err error) error {
	le.Close()

	return err
}

// rtcNAT1To1IPs splits the comma-separated INDRI_RTC_NAT_1TO1_IPS value into
// the slice webrtcTransport.Config expects. An empty string yields no IPs,
// matching envconfig's own empty-string default.
func rtcNAT1To1IPs(csv string) []string {
	if csv == "" {
		return nil
	}

	var ips []string

	for _, ip := range strings.Split(csv, ",") {
		if trimmed := strings.TrimSpace(ip); trimmed != "" {
			ips = append(ips, trimmed)
		}
	}

	return ips
}
