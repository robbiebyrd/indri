package lua

import (
	"context"
	"expvar"
	"fmt"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

// A killed script is counted under why it was killed, so an operator reading
// the counters can tell a script that is too slow from one that builds too big
// a value from one with a bug — the three need different fixes, and a single
// "script failed" count would hide which one they have.
func TestKills_AreCountedByReason(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		src     string
		timeout time.Duration
		reason  KillReason
	}{
		{
			name:    "an infinite loop runs out its deadline",
			src:     `indri.on("kill", function(req) while true do end end)`,
			timeout: 50 * time.Millisecond,
			reason:  KillDeadline,
		},
		{
			name:   "string.rep past the string cap",
			src:    `indri.on("kill", function(req) local s = string.rep("x", 1000000000) end)`,
			reason: KillStringCap,
		},
		{
			name:   "string.format past the string cap",
			src:    `indri.on("kill", function(req) local s = string.format("%999999999d", 1) end)`,
			reason: KillStringCap,
		},
		{
			name: "a reply nested past the depth cap",
			src: `indri.on("kill", function(req)
				local t = {}
				for i = 1, 40 do t = {inner = t} end
				indri.reply(t)
			end)`,
			reason: KillDepthCap,
		},
		{
			name: "a reply past the node cap",
			src: `indri.on("kill", function(req)
				local t = {}
				for i = 1, 20000 do t["k" .. i] = i end
				indri.reply(t)
			end)`,
			reason: KillNodeCap,
		},
		{
			// A cap the script caught with pcall did not kill it: the error it
			// went on to raise did.
			name: "an error raised after catching a cap",
			src: `indri.on("kill", function(req)
				pcall(string.rep, "x", 1000000000)
				error("no such move", 0)
			end)`,
			reason: KillScriptError,
		},
		{
			name: "a cap hit after catching another",
			src: `indri.on("kill", function(req)
				xpcall(function() return string.rep("x", 1000000000) end, function(e) return e end)
				local t = {}
				for i = 1, 40 do t = {inner = t} end
				indri.reply(t)
			end)`,
			reason: KillDepthCap,
		},
		{
			name:   "an error the script raised",
			src:    `indri.on("kill", function(req) error("no such move", 0) end)`,
			reason: KillScriptError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := newTestEngine(t, writeScript(t, "kill.lua", tc.src))
			published := publishedKillCount(tc.reason)

			timeout := tc.timeout
			if timeout == 0 {
				timeout = 5 * time.Second
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if _, err := splitScriptError(e.Invoke(ctx, "kill", actions.Request{})); err == nil {
				t.Fatal("the script was not killed")
			}

			counts := e.KillCounts()

			if counts[tc.reason] != 1 {
				t.Errorf("killed with %v counted as %v", tc.reason, counts)
			}

			for reason, n := range counts {
				if reason != tc.reason && n != 0 {
					t.Errorf("killed with %v but also counted %d under %v", tc.reason, n, reason)
				}
			}

			if publishedKillCount(tc.reason) <= published {
				t.Errorf("the published %v counter did not move", tc.reason)
			}
		})
	}
}

// A script that finishes is not a kill.
func TestKills_ASuccessfulCallCountsNothing(t *testing.T) {
	t.Parallel()

	e := newTestEngine(t, writeScript(t, "ok.lua", `indri.on("ok", function(req) indri.reply({fine = true}) end)`))

	if _, err := e.Invoke(context.Background(), "ok", actions.Request{}); err != nil {
		t.Fatalf("invoking: %v", err)
	}

	for reason, n := range e.KillCounts() {
		if n != 0 {
			t.Errorf("a successful call counted %d kills under %v", n, reason)
		}
	}
}

// publishedKillCount reads the process-wide counter for reason.
func publishedKillCount(reason KillReason) int64 {
	m, ok := expvar.Get(killsVar).(*expvar.Map)
	if !ok {
		return 0
	}

	n, ok := m.Get(string(reason)).(*expvar.Int)
	if !ok {
		return 0
	}

	return n.Value()
}

// A script action's payload is bounded by the transports before it is
// dispatched, and converted into Lua after; the two limits are one limit, or a
// payload a transport accepted could still be refused as a host failure.
func TestPayloadLimitsMatchTheConversionBudget(t *testing.T) {
	if actions.MaxPayloadDepth != defaultMaxDepth || actions.MaxPayloadValues != defaultMaxNodes {
		t.Errorf("payload limits %d deep / %d values, conversion budget %d deep / %d nodes",
			actions.MaxPayloadDepth, actions.MaxPayloadValues, defaultMaxDepth, defaultMaxNodes)
	}
}

// The counting is the same as well as the numbers: a payload at the transports'
// limit converts, and one past it is refused by both.
func TestPayloadLimitsCountLikeTheConversion(t *testing.T) {
	L, err := defaultState()
	if err != nil {
		t.Fatalf("building a state: %v", err)
	}

	defer L.Close()

	deep := func(levels int) map[string]interface{} {
		payload := map[string]interface{}{}
		for range levels - 1 {
			payload = map[string]interface{}{"inner": payload}
		}

		return payload
	}

	wide := func(entries int) map[string]interface{} {
		payload := make(map[string]interface{}, entries)
		for i := range entries {
			payload[fmt.Sprintf("k%d", i)] = float64(i)
		}

		return payload
	}

	for name, payload := range map[string]map[string]interface{}{
		"at the depth limit":   deep(actions.MaxPayloadDepth),
		"past the depth limit": deep(actions.MaxPayloadDepth + 1),
		"at the value limit":   wide(actions.MaxPayloadValues - 1),
		"past the value limit": wide(actions.MaxPayloadValues),
	} {
		_, convertErr := toLua(L, payload)
		checkErr := actions.CheckScriptPayload(payload)

		if (convertErr == nil) != (checkErr == nil) {
			t.Errorf("%s: the transports' check says %v, the conversion says %v", name, checkErr, convertErr)
		}
	}
}
