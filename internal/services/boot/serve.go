package boot

import (
	"context"
	"log"

	"golang.org/x/sync/errgroup"

	"github.com/robbiebyrd/indri/internal/entrypoints/http"
	"github.com/robbiebyrd/indri/internal/injector"
)

func Serve(i *injector.Injector) error {
	g, ctx := errgroup.WithContext(i.GlobalContext)

	g.Go(func() error { return http.Serve(ctx, i) })
	g.Go(func() error { return monitorGameChanges(ctx, i) })

	// The third goroutine, and the one nothing else covers for: a script's
	// indri.after writes an entry to the schedule store and this is what ever
	// reads it back. Without it every timer in every game is stored and never
	// fires. It stops on the same root-context cancellation as the other two.
	g.Go(func() error { return i.Scheduler.Run(ctx) })

	err := g.Wait()

	closeResources(i)

	if err != nil {
		return err
	}

	log.Println("server shut down cleanly")

	return nil
}

// closeResources releases long-lived clients after the serving goroutines have
// returned, so a shutdown doesn't leak the Mongo connection pool.
func closeResources(i *injector.Injector) {
	if i.Transport != nil && !i.Transport.IsClosed() {
		if err := i.Transport.Close(); err != nil {
			log.Printf("error closing transport: %v", err)
		}
	}

	// Every pooled Lua state holds its own compiled scripts and globals, and
	// only Close releases them.
	if i.ServicesInjector != nil && i.LuaEngine != nil {
		i.LuaEngine.Close()
	}

	if i.MongoDBClient != nil && i.MongoDBClient.MongoClient != nil {
		if err := i.MongoDBClient.MongoClient.Disconnect(context.Background()); err != nil {
			log.Printf("error disconnecting from MongoDB: %v", err)
		}
	}
}
