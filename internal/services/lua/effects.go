package lua

import (
	"context"
	"errors"
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

// effect is one thing a script asked the host to do to the world outside the
// game document: a reply to the caller, a broadcast to the game, a timer.
//
// None of it may happen while the script is running. indri.mutate runs its
// callback inside the store's apply closure, and mutation.Run re-runs that
// closure on every version-fence miss — up to ten times — so an effect
// performed inline would fire once per attempt, including the attempts whose
// state was thrown away. Effects are therefore queued and released afterwards,
// by the ledger below.
//
// # The delivery guarantee, per effect kind
//
// The buffer is flushed *after* the write commits, and games are stored in a
// standalone mongod (CLAUDE.md: it "does not need to be a replica set"). There
// are no multi-document transactions, so the game document and an outbox of its
// effects cannot be written in one atomic act: a crash in the window between the
// commit and the flush loses whatever had not been delivered, and re-running a
// flush would deliver some of it twice. That is not fixable at this layer, so
// the guarantee is stated per kind rather than claimed away.
//
//   - indri.reply and indri.send (story 054) are best-effort. A lost frame costs
//     one player one frame: the client's next refresh returns a full keyframe and
//     client/services/game-state-parser.ts rebuilds from it. Nothing stays wrong.
//   - indri.after, indri.at and indri.cancel (story 057) are at-least-once, and
//     this is the kind that matters — a lost timer hangs a game forever, because
//     nothing else will ever fire it. A scheduled effect must carry an
//     idempotency key (schedule.CreateEntry.IdempotencyKey, whose unique index
//     makes a second Schedule with the same key return the entry already stored;
//     see TestSchedule_IdempotencyKeyDeduplicates) so that the duplicate
//     at-least-once permits is absorbed instead of played twice.
//
// Nothing here is exactly-once, and nothing built on it may be documented as
// such. What the ledger does guarantee is narrower and worth saying exactly: an
// effect queued during a mutation is emitted once per *commit* of that mutation,
// never once per attempt.
type effect interface {
	// deliver performs the effect, after the mutation that queued it committed
	// and the handler that queued it returned.
	//
	// res is the result the invocation is building: a reply appends to its
	// Responses; an effect with nothing to say to the caller ignores it.
	//
	// Delivering must not queue another effect. The invocation that owned the
	// ledger is over by then, and a script-emitted event is dispatched as its own
	// invocation with its own ledger rather than folded into this one — see the
	// depth cap in story 054. flush reports a late arrival rather than dropping
	// it silently.
	deliver(ctx context.Context, res *actions.Result) error
}

// ledger is one invocation's buffered effects, at two levels.
//
// The two levels exist because an effect queued inside indri.mutate has a
// different fate from one queued outside it. Inside, the callback may run up to
// mutation's retry budget and only the attempt that wins the version fence
// describes what was actually stored, so those effects are held at the attempt
// level and thrown away wholesale when an attempt is: the ledger only learns
// which attempt counted when MutateResult reports committed. Outside, there is
// nothing to lose a race to, so an effect goes straight to the committed level.
//
// Order is preserved across both levels, and across the boundary between them: a
// reply queued before a mutate, one queued inside it and one queued after it are
// delivered in that order, because promoting an attempt appends it to the
// committed level in the order it was queued.
//
// It is not guarded by a mutex and must not be, for the same reason invocation
// is not: it hangs off one invocation, a *lua.LState is single-threaded, and the
// pool hands a state to exactly one invocation at a time. The only goroutine
// that can reach this is the one running the call.
type ledger struct {
	// committed is what will be delivered when the handler returns: effects
	// queued outside any mutate, plus every promoted attempt.
	committed []effect

	// attempt is what the mutate attempt currently running has queued. It is
	// either promoted onto committed or discarded, never left behind.
	attempt []effect

	// inAttempt decides which level queue writes to. A bool rather than a depth
	// counter because a nested indri.mutate on the same game is refused before it
	// can start one — see invocation.enter.
	inAttempt bool
}

// queue buffers e at whichever level is currently open.
func (l *ledger) queue(e effect) {
	if l.inAttempt {
		l.attempt = append(l.attempt, e)

		return
	}

	l.committed = append(l.committed, e)
}

// beginAttempt opens a fresh attempt level, discarding whatever the previous
// attempt queued.
//
// It is called at the top of *every* run of the apply closure, not once per
// mutate, because that is what makes a retry cost nothing: the losing attempt's
// effects describe a state the store rejected, and a reply about a state nobody
// stored is worse than no reply at all.
func (l *ledger) beginAttempt() {
	l.attempt = l.attempt[:0]
	l.inAttempt = true
}

// commitAttempt promotes the current attempt's effects to the committed level,
// in order, and closes the attempt.
//
// Called only when MutateResult reported a committed write. It cannot be
// inferred from a nil error: mutation.Run returns nil both for an aborted
// attempt and for a write, which is precisely why RunResult and MutateResult
// thread the committed flag back out.
func (l *ledger) commitAttempt() {
	l.committed = append(l.committed, l.attempt...)
	l.discardAttempt()
}

// discardAttempt drops the current attempt's effects and closes the attempt.
//
// This is the fate of every attempt that lost the version fence, of an abort
// (the script returned nil, or returned a state deep-equal to the one it was
// given) and of a mutation that errored. In all three the document was left as
// it was, so the effects describe something that did not happen.
func (l *ledger) discardAttempt() {
	l.attempt = l.attempt[:0]
	l.inAttempt = false
}

// flush delivers every committed effect, in order, into res.
//
// The buffer is drained before anything is delivered, so a second flush cannot
// re-deliver what the first one already did — the ledger holds each effect
// exactly once and hands it over exactly once. What happens to the frame after
// that is the transport's business and carries the guarantee documented on
// effect.
//
// A failed delivery does not stop the ones behind it. The effects in one
// invocation are independent — a broadcast that could not be written says
// nothing about whether a timer can be scheduled — and abandoning the rest of
// the queue would turn one lost frame into a hung game. Every error is collected
// and returned together.
func (l *ledger) flush(ctx context.Context, res *actions.Result) error {
	// An attempt still open here belonged to a mutate that never reported back.
	// Dropping it is the safe reading: the ledger was never told it committed.
	l.discardAttempt()

	pending := l.committed
	l.committed = nil

	var errs []error

	for _, e := range pending {
		if err := e.deliver(ctx, res); err != nil {
			errs = append(errs, err)
		}
	}

	// Delivery is not allowed to queue, and a queue that happened anyway would
	// otherwise sit in the buffer until the state was reused and be delivered to
	// the wrong caller — or never. Say so instead.
	if late := len(l.committed); late > 0 {
		l.committed = nil

		errs = append(errs, fmt.Errorf(
			"%d effect(s) were queued while the ledger was flushing; delivering an effect must not queue another",
			late,
		))
	}

	return errors.Join(errs...)
}

// queueEffect buffers e on the ledger of the invocation L is serving.
//
// This is the seam every host function with a side effect goes through:
// indri.reply and indri.send in story 054, the timers in 057. They call this
// instead of acting, and the ledger decides when — or whether — what they asked
// for happens.
//
// An error here means there is no call in progress on this state, which is a
// wiring failure rather than a script's mistake; the caller raises it so the
// script author sees the file and line that reached the host outside a call.
func queueEffect(L *lua.LState, e effect) error {
	inv, err := currentInvocation(L)
	if err != nil {
		return err
	}

	inv.effects.queue(e)

	return nil
}
