package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/robbiebyrd/indri/internal/services/boot"
)

func main() {
	scriptFilePath := flag.String("script", "", "The name to greet")
	flag.Parse()

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
