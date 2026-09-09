package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/robbiebyrd/indri/example/tictactoe/server/handlers/move"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/services/boot"
)

func main() {
	scriptFilePath := flag.String("script", "", "A JSON file containing the default game script.")
	flag.Parse()

	if *scriptFilePath == "" {
		dir, err := os.Getwd()
		if err != nil {
			log.Fatalf("error getting current directory: %v", err)
		}

		*scriptFilePath = dir + "/config.json"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	i, err := boot.Boot(ctx, scriptFilePath)
	if err != nil {
		log.Fatalf("could not bootstrap: %v", err)
	}

	router.RegisterHandler("ttt_move", "move", move.New(i))

	if err := boot.Serve(i); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
