# Origin Policy (TR-6) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every browser origin may connect by default. An operator who sets `INDRI_ALLOWED_ORIGINS` gets exactly that list: those origins plus requests with no `Origin` header.

**Architecture:** One function, `transport.OriginChecker`, decides for every transport (ws and graphqlws through the upgrader's `CheckOrigin`; SSE and WebRTC through `transport.Route`). It changes from "no Origin, or same host as the request, or listed" to "no Origin, or no list, or listed". Dropping the same-host rule closes DNS rebinding when a list is set.

**Tech Stack:** Go `net/http`.

**Source:** `reviews/2026-09-28-full-codebase-review.md` TR-6 (prior A #10) and the decision record. Boss,
2026-09-28: accept everything by default with an optional allowlist; a list, when set, is the whole
policy.

**Why accepting every origin is safe by default:** a player's connection carries no ambient
credentials. The server sets no cookies (checked: no `Cookie` use anywhere in `internal/` or the client),
and a client authenticates by sending `login` or a token it stores itself. So a foreign page that
opens a socket gets an anonymous connection, not the player's session. The allowlist exists for
operators who want to limit which sites can use their server at all.

---

### Task 1: `OriginChecker` implements the new policy

**Files:**
- Modify: `internal/transport/origin.go:13-41`
- Test: `internal/transport/origin_test.go:11-42`

- [ ] **Step 1: Replace `TestOriginChecker` with the new policy's tests**

```go
func TestOriginChecker_WithoutAListAcceptsEveryOrigin(t *testing.T) {
	check := transport.OriginChecker("")

	for _, origin := range []string{"", "https://evil.example", "http://game.example:5002", "http://%zz"} {
		r := httptest.NewRequest(http.MethodGet, "http://game.example:5002/", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if !check(r) {
			t.Errorf("OriginChecker(\"\") refused %q", origin)
		}
	}
}

func TestOriginChecker_WithAListAcceptsOnlyItAndNativeClients(t *testing.T) {
	check := transport.OriginChecker(" https://a.example , https://b.example,")

	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{"no origin is a native client", "", true},
		{"listed origin", "https://a.example", true},
		{"second listed origin, whitespace trimmed", "https://b.example", true},
		{"unlisted origin", "https://evil.example", false},
		// With a list, the server's own origin gets no pass: a DNS-rebinding
		// page presents exactly that. A React Native iOS client, which sends
		// it, must be listed.
		{"the server's own origin, unlisted", "http://game.example:5002", false},
		{"malformed origin", "http://%zz", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://game.example:5002/", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}

			if got := check(r); got != tc.want {
				t.Errorf("OriginChecker(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `GOCACHE=$TMPDIR/gocache go test ./internal/transport/ -run OriginChecker -v`
Expected: `WithoutAList` FAILS (refuses `https://evil.example` and the malformed origin); `WithAList` FAILS on
"the server's own origin, unlisted".

- [ ] **Step 3: Implement it.** Replace `OriginChecker` and its comment with:

```go
// OriginChecker decides which browser origins may connect. Without an
// allowlist, every origin may: connections carry no ambient credentials (no
// cookies; a client authenticates with login or a token it holds), so a
// foreign page can't act as a player by opening one. With an allowlist, only
// its origins may, plus requests with no Origin (CLI and most native
// clients). The server's own origin gets no pass then, since a DNS-rebinding
// page presents exactly that; a React Native iOS client, which sends it,
// must be listed.
func OriginChecker(allowedOrigins string) func(*http.Request) bool {
	allowed := make(map[string]struct{})

	for _, o := range strings.Split(allowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" || len(allowed) == 0 {
			return true
		}

		_, ok := allowed[origin]

		return ok
	}
}
```

Remove the now-unused `"net/url"` import.

- [ ] **Step 4: Run the transport tests**

Run: `GOCACHE=$TMPDIR/gocache go test -race ./internal/transport/...`
Expected: PASS. `TestRoute_CORS` and the SSE CORS test set a list, so they're unaffected.

- [ ] **Step 5: Commit**

```bash
git add internal/transport/origin.go internal/transport/origin_test.go
git commit -m "feat(transport): accept every origin unless an allowlist is set, which is then the whole policy"
```

---

### Task 2: Docs

**Files:** `.env.example:3`, `README.md:118`, `docs/ARCHITECTURE.md:286`, `docs/PROTOCOL.md:10-15`,
`reviews/2026-09-28-full-codebase-review.md`

- [ ] **Step 1:** Say the same thing in each place, in that file's style:
  - Empty (the default) accepts every origin.
  - A list accepts only its origins, plus clients that send no `Origin` header.
  - React Native's iOS WebSocket sends the server's own origin, so list it when you set a list.
  - Connections carry no cookies or other ambient credentials, which is why the open default doesn't let
    a foreign page act as a player.

  In `PROTOCOL.md`, replace the whole "Allowed: …" sentence and "An empty allowlist rejects all
  cross-origin browsers."
- [ ] **Step 2:** In the review, mark TR-6 "fixed per Boss's decision" with the commit.
- [ ] **Step 3: Commit**

```bash
git add .env.example README.md docs/ARCHITECTURE.md docs/PROTOCOL.md reviews/2026-09-28-full-codebase-review.md
git commit -m "docs: the origin policy is open by default, and a list is the whole policy"
```

---

### Task 3: Verify

- [ ] Run: `GOCACHE=$TMPDIR/gocache go build ./... && GOCACHE=$TMPDIR/gocache go vet ./... && GOCACHE=$TMPDIR/gocache go test -race ./...`
  Expected: no `FAIL` lines.
- [ ] Hand off with superpowers:finishing-a-development-branch.
