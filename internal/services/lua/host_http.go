package lua

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"syscall"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// The bounds one indri.http.get runs under.
//
// They are deliberately small. A game script fetches a word list or a trivia
// question; anything that needs more than this is not a thing the game loop
// should be waiting on.
const (
	// defaultHTTPTimeout bounds the whole call: connect, redirects, body.
	defaultHTTPTimeout = 5 * time.Second

	// defaultHTTPDialTimeout bounds one connection attempt, so a host that
	// blackholes packets cannot spend the whole call budget on its own.
	defaultHTTPDialTimeout = 3 * time.Second

	// defaultHTTPMaxBytes caps a response body.
	defaultHTTPMaxBytes int64 = 1 << 20 // 1 MiB

	// defaultHTTPMaxRedirects caps the redirect chain. Every hop is a fresh
	// dial and therefore a fresh chance for a name to resolve somewhere
	// internal, so the chain is bounded rather than merely guarded.
	defaultHTTPMaxRedirects = 3
)

// allowedSchemes is what a script may name. Anything else — including a scheme
// a redirect tries to switch to — is refused before a connection is opened.
var allowedSchemes = []string{"http", "https"}

// allowedContentTypes is the media types a script may receive.
//
// The check is on the media type alone, so a charset parameter is accepted and
// a type that merely starts with an allowed one ("text/plaintext") is not.
var allowedContentTypes = []string{"application/json", "text/html", "text/plain"}

// dialGuard decides whether one already-resolved address may be dialled.
type dialGuard func(network, address string) error

// httpConfig is everything the http capability is bounded by.
type httpConfig struct {
	timeout      time.Duration
	dialTimeout  time.Duration
	maxBytes     int64
	maxRedirects int

	// guard runs on every dial the transport makes. It is a field rather than a
	// direct call to refuseInternalAddress so that a test can reach a loopback
	// httptest server without the shipped guard being relaxed to let it: a
	// running server only ever gets defaultHTTPConfig, which sets
	// refuseInternalAddress and is not reachable from any config file.
	guard dialGuard
}

// defaultHTTPConfig is the configuration a real server's http capability runs
// under.
func defaultHTTPConfig() httpConfig {
	return httpConfig{
		timeout:      defaultHTTPTimeout,
		dialTimeout:  defaultHTTPDialTimeout,
		maxBytes:     defaultHTTPMaxBytes,
		maxRedirects: defaultHTTPMaxRedirects,
		guard:        refuseInternalAddress,
	}
}

// validate refuses a configuration that would make the capability unbounded.
//
// It runs at install time, which is boot, because every one of these is a cap
// that exists to stop a script hanging the server: a zero here would not fail,
// it would silently remove the bound.
func (cfg httpConfig) validate() error {
	switch {
	case cfg.timeout <= 0:
		return errors.New("the per-call timeout must be positive")
	case cfg.dialTimeout <= 0:
		return errors.New("the dial timeout must be positive")
	case cfg.maxBytes <= 0:
		return errors.New("the response byte cap must be positive")
	case cfg.maxRedirects < 0:
		return errors.New("the redirect cap cannot be negative")
	case cfg.guard == nil:
		return errors.New("there is no dial guard, so nothing would refuse an internal address")
	default:
		return nil
	}
}

// httpCapability builds the installer for indri.http.
//
// The client is built once per state and shared by every call that state
// serves, which is what makes the guard a property of the capability rather
// than of a call: there is no way to reach the transport from Lua, so there is
// no way to get a fetch out of this table that skipped it.
func httpCapability(cfg httpConfig) capabilityInstaller {
	return func(L *lua.LState) (lua.LValue, error) {
		if err := cfg.validate(); err != nil {
			return nil, err
		}

		client := newGuardedClient(cfg)

		tbl := L.NewTable()
		tbl.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
			return hostHTTPGet(L, client, cfg)
		}))

		return tbl, nil
	}
}

// hostHTTPGet is indri.http.get(url).
//
// It returns a table of status, contentType and body, and raises on anything
// else, so a script reads the happy path straight down and wraps the call in
// pcall when it wants to carry on regardless.
func hostHTTPGet(L *lua.LState, client *http.Client, cfg httpConfig) int {
	raw := L.CheckString(1)

	if err := refuseInAction(L); err != nil {
		L.RaiseError("indri.http.get: %s", err.Error())
	}

	res, err := fetch(callContext(L), client, cfg, raw)
	if err != nil {
		L.RaiseError("indri.http.get(%q): %s", raw, err.Error())
	}

	tbl := L.NewTable()
	tbl.RawSetString("status", lua.LNumber(res.status))
	tbl.RawSetString("contentType", lua.LString(res.contentType))
	tbl.RawSetString("body", lua.LString(res.body))

	L.Push(tbl)

	return 1
}

// refuseInAction reports why indri.http may not be used right now, or nil when
// it may.
//
// This is the constraint the capability exists under, not a detail of it. An
// invocation is installed on the state for exactly the length of one dispatched
// action, so its presence is how this call knows it is on a player's request
// path — where a script's reach is indri.mutate, whose callback runs inside the
// store's apply closure with the game's distributed lock held and re-runs on
// every version-fence retry. A blocking fetch there would hold the lock for the
// whole timeout, and would pay it again on each of mutation.Run's retries.
//
// Nothing sets an invocation outside Invoke, so load time and any future
// scheduled callback pass. That is the intended home for http.
func refuseInAction(L *lua.LState) error {
	if _, err := currentInvocation(L); err != nil {
		return nil
	}

	return errors.New(
		"this capability is not available while a dispatched action is running, " +
			"because a blocking fetch inside indri.mutate would hold the game lock " +
			"and be repeated on every retry; http belongs to a scheduled script",
	)
}

// callContext is the deadline this call inherits: the invocation's when there is
// one, the state's load deadline otherwise.
//
// The client carries its own timeout as well, so a state with no context at all
// is still bounded.
func callContext(L *lua.LState) context.Context {
	if ctx := L.Context(); ctx != nil {
		return ctx
	}

	return context.Background()
}

// newGuardedClient builds the only http.Client this capability can reach.
//
// Three of its settings are load-bearing and none of them is a default.
//
// Control is the SSRF guard, and it is on the dialer rather than in front of
// the request because it fires with the *already-resolved* address at the
// moment of connection, for every connection the transport opens, including one
// per redirect hop. That is what closes the DNS-rebinding window: a pre-flight
// "resolve the name, check the IP, then fetch the URL" checks an answer the
// operating system is free to re-resolve differently a moment later.
//
// Proxy is nil rather than ProxyFromEnvironment. A proxy would be the only
// address ever dialled, so the guard would inspect the proxy and the name it
// was asked to reach on the script's behalf would never be resolved here at
// all.
//
// DisableKeepAlives keeps "every request is dialled, so every request is
// guarded" literally true, rather than true only until the pool reuses a
// connection.
func newGuardedClient(cfg httpConfig) *http.Client {
	dialer := &net.Dialer{
		Timeout: cfg.dialTimeout,
		Control: func(network, address string, _ syscall.RawConn) error {
			return cfg.guard(network, address)
		},
	}

	return &http.Client{
		Timeout: cfg.timeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			DisableKeepAlives:     true,
			TLSHandshakeTimeout:   cfg.dialTimeout,
			ResponseHeaderTimeout: cfg.timeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > cfg.maxRedirects {
				return fmt.Errorf("refusing to follow more than %d redirects", cfg.maxRedirects)
			}

			return checkURL(req.URL)
		},
	}
}

// httpResponse is what a script is told about a fetch.
type httpResponse struct {
	status      int
	contentType string
	body        string
}

// fetch performs one guarded GET.
//
// The order is the security property: the URL is checked before a connection is
// opened, the guard runs on every dial the transport then makes, the content
// type is checked before a single byte of body is read, and the body itself is
// read through a cap that treats reaching it as a failure.
func fetch(ctx context.Context, client *http.Client, cfg httpConfig, raw string) (httpResponse, error) {
	target, err := url.Parse(raw)
	if err != nil {
		return httpResponse{}, fmt.Errorf("parsing the url: %w", err)
	}

	if err := checkURL(target); err != nil {
		return httpResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return httpResponse{}, fmt.Errorf("building the request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return httpResponse{}, err
	}

	defer func() { _ = resp.Body.Close() }()

	mediaType, err := responseMediaType(resp)
	if err != nil {
		return httpResponse{}, err
	}

	body, err := readCapped(resp.Body, cfg.maxBytes)
	if err != nil {
		return httpResponse{}, err
	}

	return httpResponse{status: resp.StatusCode, contentType: mediaType, body: body}, nil
}

// checkURL refuses a target a script may not name, and is applied again to
// every redirect so a chain cannot leave the schemes the first hop was held to.
func checkURL(target *url.URL) error {
	scheme := strings.ToLower(target.Scheme)

	if !slices.Contains(allowedSchemes, scheme) {
		return fmt.Errorf("refusing the scheme %q; a script may fetch %v", target.Scheme, allowedSchemes)
	}

	if target.Host == "" {
		return errors.New("the url names no host")
	}

	return nil
}

// responseMediaType refuses a response a script has no business parsing.
//
// It runs on the headers, before the body is touched, because the point is not
// to hand back the wrong bytes but to avoid pulling a megabyte of somebody's
// binary through the server to find that out.
func responseMediaType(resp *http.Response) (string, error) {
	header := resp.Header.Get("Content-Type")

	if strings.TrimSpace(header) == "" {
		return "", errors.New("the response declares no content type")
	}

	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return "", fmt.Errorf("parsing the content type %q: %w", header, err)
	}

	if !slices.Contains(allowedContentTypes, mediaType) {
		return "", fmt.Errorf("refusing the content type %q; a script may read %v", mediaType, allowedContentTypes)
	}

	return mediaType, nil
}

// readCapped reads at most maxBytes and fails if there was more.
//
// One byte over the cap is requested so that "exactly the cap" and "more than
// the cap" can be told apart. Truncating silently would hand the script a
// half-parsed document that looks like a valid short one.
func readCapped(r io.Reader, maxBytes int64) (string, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("reading the response body: %w", err)
	}

	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("the response body is over the %d byte cap", maxBytes)
	}

	return string(body), nil
}

// refuseInternalAddress is the shipped dial guard: it refuses every address
// that could only be somewhere inside the deployment.
//
// It is handed the resolved address by net.Dialer.Control at the moment of
// connection, so what it inspects is what is about to be dialled rather than
// what a name resolved to some time earlier.
func refuseInternalAddress(_, address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("refusing the unreadable address %q: %w", address, err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Control is documented to receive a resolved address. Something that is
		// not an IP is therefore a surprise, and a guard that cannot tell what it
		// is looking at refuses rather than guesses.
		return fmt.Errorf("refusing the non-IP address %q", address)
	}

	if reason := internalIPReason(ip); reason != "" {
		return fmt.Errorf("refusing to connect to %s: it is %s", ip, reason)
	}

	return nil
}

// internalIPReason names why an address is out of bounds, or returns an empty
// string when it is not.
//
// The unspecified address is listed first because 0.0.0.0 and :: mean "this
// host" to a connect(2) and would otherwise be described by whichever later
// test happened to match.
func internalIPReason(ip net.IP) string {
	switch {
	case ip.IsUnspecified():
		return "the unspecified address, which connects to this host"
	case ip.IsLoopback():
		return "a loopback address"
	case ip.IsPrivate():
		return "a private address"
	case ip.IsLinkLocalUnicast():
		return "a link-local address, which is where cloud metadata services live"
	case ip.IsLinkLocalMulticast():
		return "a link-local multicast address"
	case ip.IsInterfaceLocalMulticast():
		return "an interface-local multicast address"
	case ip.IsMulticast():
		return "a multicast address"
	default:
		return ""
	}
}
