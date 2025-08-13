package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/robbiebyrd/indri/example/tictactoe/server/handlers/move"
	"github.com/robbiebyrd/indri/internal/handlers/router"
	"github.com/robbiebyrd/indri/internal/services/boot"
)

func main() {
	scriptFilePath := flag.String("script", "", "A JSON file containing the default game script.")
	flag.Parse()

	if scriptFilePath == nil || *scriptFilePath == "" {
		dir, err := os.Getwd()
		if err != nil {
			fmt.Println("Error getting current directory:", err)
			return
		}

		*scriptFilePath = dir + "/config.json"
	}

	i, err := boot.Boot(scriptFilePath)
	if err != nil {
		panic(fmt.Errorf("could not bootstrap: %v", err))
	}

	router.RegisterHandler("ttt_move", "move", move.New(i))

	boot.Serve(i)
}
