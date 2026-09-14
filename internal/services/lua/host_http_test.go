package lua

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
	"github.com/robbiebyrd/indri/internal/models"
)

// allowOnly is the tests' stand-in for a network the shipped guard would let
// through.
//
// Every httptest server is on loopback, which refuseInternalAddress refuses and
// must go on refusing — so the happy paths need an address the guard says yes
// to, and this is the only place that is arranged. It is a deliberate allowance
// and not a relaxation: refuseInternalAddress is unchanged, is still what
// defaultHTTPConfig installs, and is still exercised end to end by
// TestHTTPGet_RefusesALoopbackServerUnderTheShippedGuard.
//
// It refuses by address rather than by anything the request carries, exactly as
// the real guard does, which is what lets a test allow one hop of a redirect
// chain and refuse the next.
func allowOnly(allowed ...string) dialGuard {
	return func(_, address string) error {
		if slices.Contains(allowed, address) {
			return nil
		}

		return fmt.Errorf("refusing to connect to %s: it is a private address", address)
	}
}

// countingServer is an httptest server that records how many requests reached
// its handler.
//
// The count is the half of a refusal that matters: an error alone does not say
// whether the request was stopped before it left the process or after the
// server had already answered it.
type countingServer struct {
	*httptest.Server

	hits atomic.Int64
}

func newCountingServer(t *testing.T, handler http.HandlerFunc) *countingServer {
	t.Helper()

	srv := &countingServer{}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.hits.Add(1)
		handler(w, r)
	}))

	t.Cleanup(srv.Close)

	return srv
}

// addr is the host:port the dial guard is handed for this server.
func (s *countingServer) addr() string {
	return s.Listener.Addr().String()
}

// textServer answers every request with the given media type and body.
func textServer(t *testing.T, mediaType, body string) *countingServer {
	t.Helper()

	return newCountingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", mediaType)
		_, _ = w.Write([]byte(body))
	})
}

// testHTTPConfig is the shipped configuration with only the dial guard swapped.
func testHTTPConfig(guard dialGuard) httpConfig {
	cfg := defaultHTTPConfig()
	cfg.guard = guard

	return cfg
}

// fetchThrough performs one fetch through a client built exactly as the capability
// builds it.
func fetchThrough(t *testing.T, cfg httpConfig, rawURL string) (httpResponse, error) {
	t.Helper()

	return fetch(context.Background(), newGuardedClient(cfg), cfg, rawURL)
}

// --- the dial guard ----------------------------------------------------------

// TestRefuseInternalAddress is the predicate every dial goes through, tested on
// its own because net.Dialer.Control hands it an address string and nothing
// else: there is no network to arrange and nothing to mock.
//
// The refused list is the deployment's own inside — the process itself, the
// pod's neighbours, the cloud metadata service on 169.254.169.254 — which is
// what a server-side request forgery is aimed at.
func TestRefuseInternalAddress(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		address string
		refused bool
	}{
		"ipv4 loopback":               {address: "127.0.0.1:80", refused: true},
		"ipv4 loopback, another host": {address: "127.0.0.53:53", refused: true},
		"ipv6 loopback":               {address: "[::1]:80", refused: true},
		"rfc1918 ten":                 {address: "10.0.0.1:80", refused: true},
		"rfc1918 172.16":              {address: "172.16.5.4:8080", refused: true},
		"rfc1918 192.168":             {address: "192.168.1.1:443", refused: true},
		"ipv6 unique local":           {address: "[fd00::1]:80", refused: true},
		"link-local, cloud metadata":  {address: "169.254.169.254:80", refused: true},
		"ipv6 link-local":             {address: "[fe80::1]:80", refused: true},
		"ipv4 multicast":              {address: "224.0.0.1:80", refused: true},
		"ipv6 link-local multicast":   {address: "[ff02::1]:80", refused: true},
		"ipv6 interface-local mcast":  {address: "[ff01::1]:80", refused: true},
		"ipv4 unspecified":            {address: "0.0.0.0:80", refused: true},
		"ipv6 unspecified":            {address: "[::]:80", refused: true},
		"a name rather than an ip":    {address: "metadata.google.internal:80", refused: true},
		"no port at all":              {address: "8.8.8.8", refused: true},
		"public ipv4":                 {address: "8.8.8.8:443", refused: false},
		"public ipv4, documentation":  {address: "203.0.113.5:80", refused: false},
		"public ipv6":                 {address: "[2001:4860:4860::8888]:443", refused: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := refuseInternalAddress("tcp", test.address)

			switch {
			case test.refused && err == nil:
				t.Fatalf("dialling %s was allowed, want it refused", test.address)
			case !test.refused && err != nil:
				t.Fatalf("dialling %s was refused (%v), want it allowed", test.address, err)
			}
		})
	}
}

// TestHTTPGet_RefusesALoopbackServerUnderTheShippedGuard is the guard end to
// end: the configuration a real server runs, against a server that really is
// listening, refusing to reach it.
//
// It is what keeps the allowance the other tests make honest. If
// refuseInternalAddress were ever loosened to make a test pass, this is the
// test that would stop passing.
func TestHTTPGet_RefusesALoopbackServerUnderTheShippedGuard(t *testing.T) {
	t.Parallel()

	srv := textServer(t, "text/plain", "secret")

	_, err := fetchThrough(t, defaultHTTPConfig(), srv.URL)
	requireErrorMentions(t, err, "loopback")

	if hits := srv.hits.Load(); hits != 0 {
		t.Fatalf("the server was reached %d times, want the connection refused before it was opened", hits)
	}
}

// --- redirects ---------------------------------------------------------------

// TestHTTPGet_RefusesARedirectWhoseNextHopIsRefused is the DNS-rebinding race,
// closed.
//
// The guard is on the dialer rather than in front of the request, so it runs
// again for the second hop's own connection. A pre-flight check on the URL the
// script named would have passed this fetch: the first hop is allowed, and
// nothing about it says where the redirect points.
//
// The second server's hit count is the assertion that matters. An error alone
// could mean the response came back and was then rejected; zero hits means the
// connection was never opened.
func TestHTTPGet_RefusesARedirectWhoseNextHopIsRefused(t *testing.T) {
	t.Parallel()

	internal := textServer(t, "text/plain", "internal secret")

	public := newCountingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	})

	_, err := fetchThrough(t, testHTTPConfig(allowOnly(public.addr())), public.URL)
	requireErrorMentions(t, err, "refusing to connect", "private address")

	if hits := internal.hits.Load(); hits != 0 {
		t.Fatalf("the second hop was reached %d times, want the redirect refused at the dial", hits)
	}

	if hits := public.hits.Load(); hits != 1 {
		t.Fatalf("the first hop was reached %d times, want exactly one", hits)
	}
}

// TestHTTPGet_RefusesTooManyRedirects bounds the chain. Every hop is a fresh
// dial and a fresh chance for a name to resolve somewhere internal, so an
// unbounded chain is an unbounded number of attempts to find one.
func TestHTTPGet_RefusesTooManyRedirects(t *testing.T) {
	t.Parallel()

	var srv *countingServer

	srv = newCountingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound)
	})

	cfg := testHTTPConfig(allowOnly(srv.addr()))
	cfg.maxRedirects = 2

	_, err := fetchThrough(t, cfg, srv.URL)
	requireErrorMentions(t, err, "more than 2 redirects")

	// The first request plus the two redirects that were allowed.
	if hits := srv.hits.Load(); hits != 3 {
		t.Fatalf("the server was reached %d times, want 3", hits)
	}
}

// TestHTTPGet_RefusesANonHTTPScheme keeps a script, and a redirect, from naming
// something the transport would treat as anything other than a web request.
func TestHTTPGet_RefusesANonHTTPScheme(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		url   string
		wants []string
	}{
		"a file url":     {url: "file:///etc/passwd", wants: []string{"refusing the scheme", "file"}},
		"an ftp url":     {url: "ftp://example.com/x", wants: []string{"refusing the scheme", "ftp"}},
		"no scheme":      {url: "example.com/x", wants: []string{"refusing the scheme"}},
		"no host at all": {url: "http:///nothing", wants: []string{"names no host"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := fetchThrough(t, testHTTPConfig(allowOnly()), test.url)
			requireErrorMentions(t, err, test.wants...)
		})
	}
}

// --- the response ------------------------------------------------------------

// TestHTTPGet_ReturnsATextResponse is the happy path, and the reason the rest
// of these tests are refusals rather than a capability that simply never works.
func TestHTTPGet_ReturnsATextResponse(t *testing.T) {
	t.Parallel()

	srv := textServer(t, "application/json; charset=utf-8", `{"word":"otter"}`)

	res, err := fetchThrough(t, testHTTPConfig(allowOnly(srv.addr())), srv.URL)
	if err != nil {
		t.Fatalf("fetching %s: %v", srv.URL, err)
	}

	if res.status != http.StatusOK {
		t.Fatalf("the status is %d, want %d", res.status, http.StatusOK)
	}

	// The parameter is parsed off, so a charset does not turn an allowed media
	// type into an unrecognised one.
	if res.contentType != "application/json" {
		t.Fatalf("the content type is %q, want %q", res.contentType, "application/json")
	}

	if res.body != `{"word":"otter"}` {
		t.Fatalf("the body is %q, want the server's", res.body)
	}
}

// TestHTTPGet_RefusesANonTextContentTypeBeforeReadingTheBody proves the order,
// not merely the refusal.
//
// The server sends its headers and then stops, releasing the body only when the
// test lets it. A fetch that read first and checked afterwards would block
// there until the call timed out, and the error would name the timeout rather
// than the content type.
func TestHTTPGet_RefusesANonTextContentTypeBeforeReadingTheBody(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})

	srv := newCountingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		select {
		case <-release:
		case <-r.Context().Done():
		}

		_, _ = w.Write([]byte("binary"))
	})

	// Before the server's own cleanup, which waits for the handler to return.
	t.Cleanup(func() { close(release) })

	_, err := fetchThrough(t, testHTTPConfig(allowOnly(srv.addr())), srv.URL)
	requireErrorMentions(t, err, "refusing the content type", "application/octet-stream")
}

// TestHTTPGet_RefusesAResponseWithNoContentType closes the gap a missing header
// would otherwise open: nothing to check is not the same as nothing to worry
// about.
func TestHTTPGet_RefusesAResponseWithNoContentType(t *testing.T) {
	t.Parallel()

	srv := newCountingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header()["Content-Type"] = nil
		w.WriteHeader(http.StatusOK)
	})

	_, err := fetchThrough(t, testHTTPConfig(allowOnly(srv.addr())), srv.URL)
	requireErrorMentions(t, err, "no content type")
}

// TestHTTPGet_TreatsTheByteCapAsAnError is the difference between a cap and a
// truncation. A silently short body is a document that parses and is wrong,
// which is worse than a fetch that failed.
func TestHTTPGet_TreatsTheByteCapAsAnError(t *testing.T) {
	t.Parallel()

	const bodyCap = 16

	tests := map[string]struct {
		size    int
		refused bool
	}{
		"under the cap":   {size: bodyCap - 1},
		"exactly the cap": {size: bodyCap},
		"one byte over":   {size: bodyCap + 1, refused: true},
		"far over":        {size: bodyCap * 100, refused: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body := strings.Repeat("x", test.size)
			srv := textServer(t, "text/plain", body)

			cfg := testHTTPConfig(allowOnly(srv.addr()))
			cfg.maxBytes = bodyCap

			res, err := fetchThrough(t, cfg, srv.URL)

			if test.refused {
				requireErrorMentions(t, err, "over the 16 byte cap")

				return
			}

			if err != nil {
				t.Fatalf("fetching a %d byte body under a %d byte cap: %v", test.size, bodyCap, err)
			}

			if res.body != body {
				t.Fatalf("the body is %d bytes, want the server's %d", len(res.body), test.size)
			}
		})
	}
}

// TestHTTPGet_TimesOut is the bound on a server that accepts a connection and
// then says nothing, which no other check here would ever fire on.
func TestHTTPGet_TimesOut(t *testing.T) {
	t.Parallel()

	srv := newCountingServer(t, func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	cfg := testHTTPConfig(allowOnly(srv.addr()))
	cfg.timeout = 100 * time.Millisecond

	started := time.Now()

	_, err := fetchThrough(t, cfg, srv.URL)
	if err == nil {
		t.Fatal("fetching from a server that never answers returned no error, want a timeout")
	}

	// Loose, because the assertion is that the timeout is what ended the call
	// rather than the test's own patience.
	if elapsed := time.Since(started); elapsed > testTimeout {
		t.Fatalf("the call took %v, want it bounded by the %v timeout", elapsed, cfg.timeout)
	}
}

// --- the capability as a script sees it --------------------------------------

// newHTTPEngine builds an engine over one script that was granted http, with
// http as the only capability that exists.
//
// It goes through newEngine rather than NewEngineWithGrants so the test decides
// what the capability is bounded by: the shipped configuration is what
// defaultCapabilities installs, and a test cannot reach a loopback server under
// it.
func newHTTPEngine(t *testing.T, cfg httpConfig, games GameMutator, src string) (*Engine, error) {
	t.Helper()

	path := writeScript(t, "game.lua", src)

	e, err := newEngine(
		[]models.ScriptFile{granted(path, CapabilityHTTP)},
		games,
		// Off the action view, exactly as defaultCapabilities declares it:
		// TestDefaultCapabilities_KeepHTTPOffTheActionView is what holds the two
		// declarations together.
		capabilitySet{CapabilityHTTP: offTheActionView(httpCapability(cfg))},
	)

	if e != nil {
		t.Cleanup(e.Close)
	}

	return e, err
}

// TestHTTPCapability_IsUsableOutsideADispatchedAction is the other half of the
// action view: without it, "http is absent from an action" would also be true
// of an http capability that never worked at all.
//
// Load time is where a chunk runs under its script's full view, which is what a
// capability granted to a script has to be reachable from — the alternative is a
// grant that is silently inert.
func TestHTTPCapability_IsUsableOutsideADispatchedAction(t *testing.T) {
	t.Parallel()

	srv := textServer(t, "text/plain", "otter")

	_, err := newHTTPEngine(t, testHTTPConfig(allowOnly(srv.addr())), nil, `
local res = indri.http.get("`+srv.URL+`")

assert(res.status == 200, "status " .. tostring(res.status))
assert(res.body == "otter", "body " .. tostring(res.body))
assert(res.contentType == "text/plain", "content type " .. tostring(res.contentType))

indri.on("noop", function(req) end)
`)
	if err != nil {
		t.Fatalf("loading a script that fetches: %v", err)
	}

	if hits := srv.hits.Load(); hits == 0 {
		t.Fatal("the server was never reached, want the script's fetch to have arrived")
	}
}

// dispatchMove dispatches the "move" action as a player in a game, and returns
// whatever the script made of it.
//
// It is invokeMove's counterpart for an engine the test has already built, which
// is what a granted script needs: what it was granted is decided at
// construction.
func dispatchMove(t *testing.T, e *Engine, gameID string) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, err := splitScriptError(e.Invoke(ctx, "move", actions.Request{
		Session: &models.Session{UserID: stringPtr("player-1"), GameID: &gameID},
	}))

	return err
}

// TestHTTPCapability_IsAbsentFromAnActionHandler is the constraint this
// capability lives under, and the form the constraint takes.
//
// A dispatched action is a player waiting on a request, and the only way a
// script changes anything from there is indri.mutate, whose callback runs inside
// the store's apply closure with the game's lock held and is re-run on every
// version-fence retry. A blocking fetch in that closure would hold the lock for
// the whole timeout and pay it again on each retry.
//
// What the script sees is nil, not a function that refuses. The difference is
// the point: a capability that is not on the table cannot be reached by moving
// the call somewhere the check does not run, and there is no function body to
// audit to find that out.
//
// Both places a handler could read it from are covered — the handler itself, and
// the mutate callback inside it, which inherits the handler's environment.
func TestHTTPCapability_IsAbsentFromAnActionHandler(t *testing.T) {
	t.Parallel()

	store, _, gameID := newTestGame(t)
	srv := textServer(t, "text/plain", "otter")

	e, err := newHTTPEngine(t, testHTTPConfig(allowOnly(srv.addr())), store, `
indri.on("move", function(req)
  assert(indri.http == nil, "indri.http is on the action view")
  assert(_G.indri.http == nil, "_G.indri.http is reachable from an action")
  assert(type(indri.mutate) == "function", "indri.mutate is missing from the action view")

  indri.mutate(function(state)
    assert(indri.http == nil, "indri.http is reachable from inside indri.mutate")

    return nil
  end)
end)
`)
	if err != nil {
		t.Fatalf("building an engine: %v", err)
	}

	if err := dispatchMove(t, e, gameID); err != nil {
		t.Fatalf("dispatching an action to a script granted http: %v", err)
	}
}

// TestHTTPCapability_CannotBeFetchedThroughFromAnActionHandler is the same claim
// stated as a script author would hit it: a handler that calls indri.http.get
// fails on the nil, and no request leaves the process.
//
// The hit count is what an error alone would not say. A capability that raised
// after fetching, or one whose refusal arrived after the connection was opened,
// would satisfy the error and still have reached somebody else's server from
// inside a game lock.
func TestHTTPCapability_CannotBeFetchedThroughFromAnActionHandler(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"straight from the handler": `
indri.on("move", function(req)
  indri.http.get("%s")
end)
`,
		"from inside indri.mutate": `
indri.on("move", function(req)
  indri.mutate(function(state)
    indri.http.get("%s")

    return state
  end)
end)
`,
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store, _, gameID := newTestGame(t)
			srv := textServer(t, "text/plain", "otter")

			e, err := newHTTPEngine(t, testHTTPConfig(allowOnly(srv.addr())), store, fmt.Sprintf(src, srv.URL))
			if err != nil {
				t.Fatalf("building an engine: %v", err)
			}

			// The nil is indri.http itself, so the failure is the index that
			// followed it rather than anything inside the capability.
			requireErrorMentions(t, dispatchMove(t, e, gameID), "attempt to index", "get")

			// The guard would let this server through, so a reachable capability
			// would have fetched from it.
			if hits := srv.hits.Load(); hits != 0 {
				t.Fatalf("the server was reached %d times from an action, want none", hits)
			}
		})
	}
}

// TestHTTPCapability_ReachesALifecycleHandler is where http was always meant to
// live, and the reason absence from an action is a narrowing rather than a ban.
//
// A lifecycle handler runs on the queue's own goroutine, after the write that
// raised it committed, with no game lock of its own and nobody waiting: the
// fetch costs this handler its deadline and nothing else. What it fetched is
// written into the game through indri.mutate and read back out of the store, so
// a handler that never ran and a handler that ran and fetched nothing are not
// the same result.
func TestHTTPCapability_ReachesALifecycleHandler(t *testing.T) {
	t.Parallel()

	store, _, gameID := newTestGame(t)
	srv := textServer(t, "text/plain", "otter")

	e, err := newHTTPEngine(t, testHTTPConfig(allowOnly(srv.addr())), store, fmt.Sprintf(`
indri.on("player:joined", function(ev)
  local res = indri.http.get(%q)

  indri.mutate(function(state)
    state.data.fetched = res.body

    return state
  end)
end)

indri.on("move", function(req) end)
`, srv.URL))
	if err != nil {
		t.Fatalf("building an engine: %v", err)
	}

	e.EmitLifecycle(LifecyclePlayerJoined, gameID, nil)

	g, err := store.Get(gameID)
	if err != nil {
		t.Fatalf("reading the game back: %v", err)
	}

	if fetched, _ := g.PublicData["fetched"].(string); fetched != "otter" {
		t.Fatalf("the lifecycle handler recorded %q, want the body it fetched", fetched)
	}

	if hits := srv.hits.Load(); hits != 1 {
		t.Fatalf("the server was reached %d times, want exactly one fetch", hits)
	}
}

// TestHTTPCapability_RefusesAnUnboundedConfiguration fails the install, and so
// boot, rather than installing a capability with a bound missing. A zero here
// does not behave badly, it behaves without a limit, which is the one failure
// mode nothing downstream would notice.
func TestHTTPCapability_RefusesAnUnboundedConfiguration(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		spoil func(*httpConfig)
		wants []string
	}{
		"no call timeout": {spoil: func(c *httpConfig) { c.timeout = 0 }, wants: []string{"per-call timeout"}},
		"no dial timeout": {spoil: func(c *httpConfig) { c.dialTimeout = 0 }, wants: []string{"dial timeout"}},
		"no byte cap":     {spoil: func(c *httpConfig) { c.maxBytes = 0 }, wants: []string{"byte cap"}},
		"negative hops":   {spoil: func(c *httpConfig) { c.maxRedirects = -1 }, wants: []string{"redirect cap"}},
		"no dial guard":   {spoil: func(c *httpConfig) { c.guard = nil }, wants: []string{"dial guard"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg := defaultHTTPConfig()
			test.spoil(&cfg)

			_, err := newHTTPEngine(t, cfg, nil, `indri.on("noop", function(req) end)`)
			requireErrorMentions(t, err, append(test.wants, CapabilityHTTP)...)
		})
	}
}
