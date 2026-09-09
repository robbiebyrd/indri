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

	if i.MongoDBClient != nil && i.MongoDBClient.MongoClient != nil {
		if err := i.MongoDBClient.MongoClient.Disconnect(context.Background()); err != nil {
			log.Printf("error disconnecting from MongoDB: %v", err)
		}
	}
}
