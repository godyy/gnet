package main

import (
	"flag"
	"fmt"
	"github.com/godyy/gnet"
	"github.com/godyy/gnet/internal/examples/chat"
	"github.com/godyy/gnet/internal/examples/chat/server"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	servPort := flag.String("serv-port", "8822", "specify service port")

	opt := gnet.NewTCPSessionOption()
	opt.SetReceiveTimeout(chat.ReceiveTimeout)
	opt.SetSendTimeout(chat.SendTimeout)
	servAddr := fmt.Sprintf(":%s", *servPort)
	s := server.NewServer()
	if err := s.Start(servAddr, opt); err != nil {
		log.Fatalf("server start: %v", err)
	} else {
		log.Printf("server started, listening at %s", servAddr)
	}

	chStop := make(chan os.Signal, 1)
	signal.Notify(chStop, syscall.SIGINT, syscall.SIGTERM)
	_, _ = <-chStop
}
