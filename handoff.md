# Handoff: main divergence integration

You are continuing an in-flight branch integration on the Indri repo. Read
[CLAUDE.md](CLAUDE.md) first — it is authoritative and its rules override defaults.

## Situation

Local `main` and `origin/main` diverged from common ancestor `26a633a` with no shared
history since: 105 local commits against 201 remote, 132 merge conflicts, 73 of them
`add/add`. A plain merge is not viable.

**Decision already made, and not up for revisiting: local `main` is the trunk.**
`origin/main` never received the connection-independent dispatch refactor — its handlers
still take `transport.Conn`, while 50 local files depend on `actions.Request`. The two
sides diverged on orthogonal axes: remote is newer on storage, older on dispatch. So
remote's storage, slot model and wire format port *onto* local, not the reverse.

- Plan: [plans/main-divergence-integration.md](plans/main-divergence-integration.md)
- Branch: `POC-00003/main-integration` (5 commits; build, vet and gofmt all clean)
- Regression baseline: [.integration/baseline.txt](.integration/baseline.txt)

## Done

| Story | Work |
|---|---|
| 078 | Verified baseline on the integration branch |
| 079 | SQLite and Postgres clients, `repo/ids`, repo sentinel errors |
| 088 | Model IDs moved from `bson.ObjectID` to string |
| 080 | SQLite and Postgres game backends |

## Next

Story **081** — the player slot model.

```bash
wiz-run stories-cli.js show 081-7e0a    # its worklog carries a handoff note
```

Then 082, 084, 086, 083, 085, 087. Story 084 is independent of the storage chain and can
run at any time.

## How to work

One story at a time: `status in_progress` → worklog → check criteria → qa → proof gate →
`complete` → commit with `closes #<id>`.

TDD is expected. Write the failing test first, and show it failing.

Start the databases, or tests silently skip and report green without running:

```bash
docker compose up -d mongodb
docker compose --profile postgres up -d postgres
export INDRI_TEST_POSTGRES_URI='postgres://indri:indri@localhost:5432/indri?sslmode=disable'
```

Verify with `go build ./... && go vet ./... && go test -race -count=1 ./...`. Use
`-count=1`; cached results have masked problems here. Client checks must mirror CI: delete
`node_modules`, then `pnpm install --frozen-lockfile && pnpm run typecheck && pnpm test`.

## Things that will bite you

**A passing suite is not proof.** The contract suite passed against a deliberately broken
version fence, because `mutation.Run` holds a lock across the whole read-modify-write, so
no test ever interleaves two writers. `internal/repo/game/fence_contract_test.go` now
covers that by reaching the `docs` port directly. When you add a backend behaviour, break
it on purpose and confirm a test fails. If nothing fails, the test is worthless.

**Four backends implement `docs`** — `mongoDocs` (game.go), `memoryDocs` (memory.go) and
`sqlDocs` (sql.go, serving both SQL dialects). Any new `Storer` method lands on all four
plus the `var _ Storer` assertions in interface.go, or the build breaks.

**Two different values are both called `sessionId`.** The connection key carries
`Session.ID` and is never sent to clients; the wire field carries `Session.Token`.
Authorisation resolves the caller from their own connection, never a client-supplied id.
Story 081 touches this — keep it.

**No backward compatibility without explicit approval** (CLAUDE.md). The ID change already
broke existing MongoDB documents by design; drop the local `indri` database rather than
writing a shim.

**`internal/repo/schedule` still uses `bson.ObjectID`** deliberately. It is Mongo-only.
Do not "fix" it as part of another story.

**Scope your file globs.** An over-broad `rglob` once edited files inside
`.claude/worktrees/` — a separate git worktree on another branch.

## Open items needing Robbie's decision — ask, do not assume

- **Commits are unsigned.** `commit.gpgsign=false` globally and `user.signingkey` is unset,
  and the whole repo history is unsigned, but CLAUDE.md requires signing. A key exists
  (`FC08DF21B41D9E56`) but it is a smartcard needing a PIN.
- **`cff6b5f` was committed directly to `main`** before the branch was cut. Recoverable by
  moving `main` back and rebasing.
- **Story 083 (compact delta) has measured evidence and no decision.** No compression is
  enabled anywhere; enabling permessage-deflate gets 85% off, while the whole compact-delta
  migration adds only 30% more, because positional paths and deflate attack the same
  redundancy. The keyframe split (4936 → 754 bytes) is the part genuinely worth taking and
  is orthogonal to it. Unresolved: whether React Native negotiates permessage-deflate — if
  it does not, the raw numbers matter and the answer may flip. Reproduction scripts are in
  `.integration/scratch/`.
- **Story 079 has a stale criterion line.** Criterion 5 reads `- [x]` but still carries the
  rejection text from before it was proven. The CLI cannot amend criterion text, and
  CLAUDE.md forbids hand-editing story files.

Report honestly. If something is skipped, say so. If a criterion cannot be proven, reject
it with the reason rather than checking the box.
