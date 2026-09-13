package boot

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

// Serve cannot be called from a test — it binds a listener and needs a live
// Mongo pool — so what it launches is read off the source instead, the way
// resolverDispatches reads the GraphQL resolvers.
//
// The test exists because the failure it catches is silent. A scheduler that is
// built, wired to the engine and never run leaves every indri.after in every
// script writing entries nothing ever reads: the game simply stops, with no
// error anywhere. Nothing else in the suite would notice, because every other
// test drives the loop directly.
func TestServe_LaunchesTheSchedulerBesideTheOtherTwoGoroutines(t *testing.T) {
	launched := goroutinesLaunchedBy(t, "Serve")

	if len(launched) != 3 {
		t.Fatalf("Serve launches %d goroutines, want 3: %v", len(launched), launched)
	}

	wants := map[string]string{
		"the http server":           "http.Serve(ctx, i)",
		"the change broadcaster":    "monitorGameChanges(ctx, i)",
		"the scheduled-action loop": "i.Scheduler.Run(ctx)",
	}

	for name, call := range wants {
		found := false

		for _, body := range launched {
			if strings.Contains(body, call) {
				found = true

				break
			}
		}

		if !found {
			t.Errorf("Serve does not run %s (%s); it launches %v", name, call, launched)
		}
	}

	// Every one of them takes the errgroup's context, which is what makes a
	// SIGINT reach all three rather than two of them.
	for _, body := range launched {
		if !strings.Contains(body, "ctx") {
			t.Errorf("a goroutine Serve launches ignores the shutdown context: %s", body)
		}
	}
}

// goroutinesLaunchedBy returns the source of each `g.Go(...)` argument inside
// the named function in serve.go.
func goroutinesLaunchedBy(t *testing.T, funcName string) []string {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "serve.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing serve.go: %v", err)
	}

	var launched []string

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName {
			continue
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Go" {
				return true
			}

			if ident, ok := sel.X.(*ast.Ident); !ok || ident.Name != "g" {
				return true
			}

			var buf bytes.Buffer

			if err := printer.Fprint(&buf, fset, call.Args[0]); err != nil {
				t.Fatalf("rendering a goroutine body: %v", err)
			}

			launched = append(launched, buf.String())

			return true
		})
	}

	return launched
}
