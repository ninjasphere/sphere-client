package main

import (
	"os"
	"os/signal"

	"github.com/ninjasphere/go-ninja/logger"
	"github.com/ninjasphere/sphere-client/client"
)

func main() {

	client.NewClient()

	s := make(chan os.Signal, 1)
	signal.Notify(s, os.Interrupt, os.Kill)
	logger.GetLogger("Client").Infof("Got signal: %v", <-s)
}
