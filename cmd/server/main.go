package main

import (
	melodyClient "github.com/robbiebyrd/indri/internal/clients/melody"
	"github.com/robbiebyrd/indri/internal/entrypoints"
	"github.com/robbiebyrd/indri/internal/handlers/message"
	"github.com/robbiebyrd/indri/internal/services/boot"

	"log"
)

func main() {
	m, err := melodyClient.New()
	if err != nil {
		log.Fatal(err)
	}

	err = boot.Register()
	if err != nil {
		log.Fatal(err)
	}

	m.HandleConnect(entrypoints.HandleConnect)
	m.HandleDisconnect(entrypoints.HandleDisconnect)
	m.HandleMessage(message.HandleMessage)
	entrypoints.Serve()
}
