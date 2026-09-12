package rest

import (
	"fmt"
	"slices"
)

// build turns a decoded request body into the payload an action handler expects,
// rejecting the request when a required argument is missing. Returning an error
// here keeps a malformed request away from the handlers entirely.
type build func(map[string]interface{}) (map[string]interface{}, error)

// routes is the action surface. Each entry mirrors the equivalent GraphQL
// mutation: the same action name, the same payload keys, the same required
// arguments. Adding an action means adding it here, as a mutation, and as a
// WebSocket handler — the three inbound paths are independent.
var routes = map[string]build{
	"register": args(required("email", "password", "name")),
	"login":    args(required("email", "password")),

	// The reconnect handler reads the token from "sessionId", so the field is
	// renamed on the way in. Callers send the friendlier "token".
	"reconnect": func(in map[string]interface{}) (map[string]interface{}, error) {
		token, err := requiredString(in, "token")
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{"sessionId": token}, nil
	},

	"create": func(in map[string]interface{}) (map[string]interface{}, error) {
		out, err := args(required("code", "teamId"))(in)
		if err != nil {
			return nil, err
		}

		private, ok := in["private"].(bool)
		if !ok && in["private"] != nil {
			return nil, fmt.Errorf("%q must be a boolean", "private")
		}

		out["private"] = private

		return out, nil
	},

	"join":   args(required("code", "teamId")),
	"leave":  args(),
	"kick":   args(required("code", "userId")),
	"logout": args(),

	// The keyframe route. An SSE client cannot build state from deltas alone,
	// so it POSTs here first and replays its stream over the result.
	"refresh": args(),

	"inquire": args(required("inquiryType"), optional("inquiry", "code")),

	// The layout route cannot use args(): an op's arguments are objects whose
	// shape depends on the op, not a fixed list of strings. The body is passed
	// through whole and the handler's decodeOp rejects every field the op does
	// not declare, so a caller still cannot smuggle an unknown key past it.
	"layout": func(in map[string]interface{}) (map[string]interface{}, error) {
		for _, name := range []string{"code", "op"} {
			if _, err := requiredString(in, name); err != nil {
				return nil, err
			}
		}

		return in, nil
	},
}

// Actions returns every action the REST API exposes, sorted. It exists so a
// test can assert this table has not drifted from the handler registry: an
// action reachable over WebSocket but not over REST is a silent gap.
func Actions() []string {
	names := make([]string, 0, len(routes))
	for action := range routes {
		names = append(names, action)
	}

	slices.Sort(names)

	return names
}

type argSpec struct {
	required []string
	optional []string
}

type argOption func(*argSpec)

func required(names ...string) argOption {
	return func(s *argSpec) { s.required = append(s.required, names...) }
}

func optional(names ...string) argOption {
	return func(s *argSpec) { s.optional = append(s.optional, names...) }
}

// args builds a validator that copies the named string arguments across and
// drops everything else, so a caller cannot smuggle extra keys into a payload.
func args(options ...argOption) build {
	var spec argSpec
	for _, option := range options {
		option(&spec)
	}

	return func(in map[string]interface{}) (map[string]interface{}, error) {
		out := make(map[string]interface{}, len(spec.required)+len(spec.optional))

		for _, name := range spec.required {
			value, err := requiredString(in, name)
			if err != nil {
				return nil, err
			}

			out[name] = value
		}

		for _, name := range spec.optional {
			if in[name] == nil {
				continue
			}

			value, ok := in[name].(string)
			if !ok {
				return nil, fmt.Errorf("%q must be a string", name)
			}

			out[name] = value
		}

		return out, nil
	}
}

func requiredString(in map[string]interface{}, name string) (string, error) {
	value, ok := in[name].(string)
	if !ok {
		return "", fmt.Errorf("%q is required and must be a string", name)
	}

	if value == "" {
		return "", fmt.Errorf("%q must not be empty", name)
	}

	return value, nil
}
