package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/godyy/gnet/internal/examples/chat/server"
)

func main() {
	servPort := flag.String("serv-port", "8822", "specify service port")

	servAddr := fmt.Sprintf(":%s", *servPort)
	s := server.NewServer()
	if err := s.Start(servAddr); err != nil {
		log.Fatalf("server start: %v", err)
	} else {
		log.Printf("server started, listening at %s", servAddr)
	}

	chStop := make(chan os.Signal, 1)
	signal.Notify(chStop, syscall.SIGINT, syscall.SIGTERM)
	_, _ = <-chStop
}
