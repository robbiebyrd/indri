package main

import (
	"flag"
	"fmt"
	"github.com/robbiebyrd/indri/internal/services/boot"
)

func main() {
	scriptFilePath := flag.String("script", "", "The name to greet")
	flag.Parse()

	i, err := boot.Boot(scriptFilePath)
	if err != nil {
		panic(fmt.Errorf("could not bootstrap: %v", err))
	}

	boot.Start(i)
}
