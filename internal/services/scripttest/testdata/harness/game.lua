-- A script that exists to be tested by the harness's own tests, not to be a
-- game. Each handler is the smallest thing that produces one of the outcomes a
-- fixture has to be able to describe.

-- fault raises without saying so: it indexes a nil, which is the shape of an
-- author's own bug rather than a deliberate refusal. gopher-lua prefixes the
-- message with this file and the line, which is the whole reason this handler
-- exists -- a fixture that fails here must be able to point at a line.
indri.on("fault", function(_)
    local missing = nil

    return missing.field
end)

-- greet answers its caller and writes nothing, which is what indri.reply is
-- for.
indri.on("greet", function(_)
    indri.reply({ hello = "world" })
end)

-- idle does nothing at all: no write, no reply. A handler that correctly
-- decided there was nothing to do publishes no delta, and a fixture says so
-- with "published": {}.
indri.on("idle", function(_)
end)

-- half_done commits and then raises, which is the one shape a refusal is not
-- allowed to have. indri.mutate has already saved and published by the time the
-- error leaves this handler, so its caller is told the action failed while every
-- other player has already seen it happen. A fixture expecting a refusal here
-- must notice both the delta and the version the game moved to.
indri.on("half_done", function(_)
    indri.mutate(function(state)
        state.data = state.data or {}
        state.data.note = "committed"

        return state
    end)

    error("but then it failed", 0)
end)

-- note writes one field, the smallest thing that publishes a delta.
indri.on("note", function(req)
    indri.mutate(function(state)
        state.data = state.data or {}
        state.data.note = (req.payload or {}).text

        return state
    end)
end)
