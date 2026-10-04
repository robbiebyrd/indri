-- A script whose whole contribution is hooks on built-in actions: it declares
-- no action and no lifecycle event of its own.
indri.before("join", function(req) end)
indri.after_action("leave", function(req) end)
