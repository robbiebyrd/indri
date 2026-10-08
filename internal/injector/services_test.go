package injector

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	gameRepo "github.com/robbiebyrd/indri/internal/repo/game"
	scheduleRepo "github.com/robbiebyrd/indri/internal/repo/schedule"
	scriptRepo "github.com/robbiebyrd/indri/internal/repo/script"
	sessionRepo "github.com/robbiebyrd/indri/internal/repo/session"
	userRepo "github.com/robbiebyrd/indri/internal/repo/user"
	"github.com/robbiebyrd/indri/internal/services/events"
	"github.com/robbiebyrd/indri/internal/services/lock"
	"github.com/robbiebyrd/indri/internal/transport/ws"
)

// The script engine is built after the transports, so the actions its scripts
// declared reach REST and GraphQL only if the services injector hands the
// manifest over. Without that, a game's moves work over WebSocket and silently
// do not exist for an SSE or GraphQL client.
func TestGetServices_ScriptActionsReachRESTAndGraphQL(t *testing.T) {
	// The WebRTC transport binds its UDP port when it is built; any free one
	// will do here.
	t.Setenv("INDRI_RTC_UDP_PORT", "0")

	if _, err := envVars.Load(nil); err != nil {
		t.Fatalf("loading settings: %v", err)
	}

	t.Cleanup(func() { _, _ = envVars.Load(nil) })

	ctx := context.Background()

	scripts, err := scriptRepo.NewStore("../../example/tictactoe/config.json")
	if err != nil {
		t.Fatalf("loading the tictactoe config: %v", err)
	}

	clients := &ClientsInjector{Transport: ws.New(), LockManager: lock.NewInProcess(), Publisher: events.NewInProcess()}
	repos := &ReposInjector{
		GameRepo:     gameRepo.NewMemoryStore(ctx, clients.LockManager, clients.Publisher),
		UserRepo:     userRepo.NewMemoryStore(),
		SessionRepo:  sessionRepo.NewMemoryStore(),
		ScriptRepo:   scripts,
		ScheduleRepo: scheduleRepo.NewMemoryStore(scheduleRepo.Config{}),
	}

	services, err := GetServices(ctx, clients, repos)
	if err != nil {
		t.Fatalf("building the services: %v", err)
	}

	t.Cleanup(func() {
		_ = clients.Transport.Close()
		services.LuaEngine.Close()
	})

	mux := http.NewServeMux()
	clients.Transport.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	resp, err := http.Post(server.URL+"/api/move", "application/json", strings.NewReader(`{"row":0,"col":0}`))
	if err != nil {
		t.Fatalf("posting the move: %v", err)
	}

	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		t.Error("POST /api/move is not routed: the script's action never reached REST")
	}

	resp, err = http.Post(server.URL+"/graphql", "application/json",
		strings.NewReader(`{"query":"mutation { scriptAction(name: \"move\", payload: {row: 0, col: 0}) }"}`))
	if err != nil {
		t.Fatalf("calling scriptAction: %v", err)
	}
	defer resp.Body.Close()

	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the answer: %v", err)
	}

	if strings.Contains(string(answer), "not an action the game scripts declared") {
		t.Errorf("scriptAction refused move: the manifest never reached GraphQL (%s)", answer)
	}
}
