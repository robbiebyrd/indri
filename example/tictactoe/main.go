package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/robbiebyrd/indri/example/tictactoe/server/handlers/move"
	"github.com/robbiebyrd/indri/example/tictactoe/server/handlers/restart"
	"github.com/robbiebyrd/indri/internal/cli"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/services/boot"
)

func main() {
	scriptFilePath, _, err := cli.Parse(flag.CommandLine, os.Args[1:], "config.json")
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	i, err := boot.Boot(ctx, &scriptFilePath)
	if err != nil {
		log.Fatalf("could not bootstrap: %v", err)
	}

	router.RegisterHandler("ttt_move", "move", move.New(i))
	router.RegisterHandler("ttt_restart", "restart", restart.New(i))

	if err := boot.Serve(i); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
