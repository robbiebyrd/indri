package boot

import (
	"log"

	"golang.org/x/sync/errgroup"

	"github.com/robbiebyrd/indri/internal/entrypoints/http"
	"github.com/robbiebyrd/indri/internal/injector"
)

func Serve(i *injector.Injector) {
	g, ctx := errgroup.WithContext(i.GlobalContext)

	g.Go(func() error { return http.Serve(i) })
	g.Go(func() error { return monitorGameChanges(ctx, i) })

	if err := g.Wait(); err != nil {
		log.Printf("One or more goroutines failed: %v\n", err)
	} else {
		log.Println("All goroutines completed successfully.")
	}
}
