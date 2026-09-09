package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"flag"

	"github.com/robbiebyrd/indri/internal/services/boot"
)

func main() {
	scriptFilePath := flag.String("script", "", "The name to greet")
	flag.Parse()

	// Cancel the root context on SIGINT/SIGTERM so the server can drain and
	// shut down cleanly instead of being killed mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	i, err := boot.Boot(ctx, scriptFilePath)
	if err != nil {
		log.Fatalf("could not bootstrap: %v", err)
	}

	if err := boot.Serve(i); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
