package main

import (
	"fmt"
	"github.com/robbiebyrd/indri/internal/repo/script"
	"github.com/robbiebyrd/indri/internal/services/boot"
)

func main() {
	gameScript := script.Get("./config.json")
	_, err := boot.Boot(gameScript)
	if err != nil {
		panic(fmt.Errorf("could not start: %v", err))
	}

	boot.Start()
}
