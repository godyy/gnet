package main

import (
	"errors"
	"flag"
	"github.com/godyy/gnet/internal/examples/iotest/define"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/godyy/gnet"
)

var (
	serverAddr    = flag.String("server-addr", ":2222", "server address")
	readBufSize   = flag.Int("read-buf-size", 1024*10, "read buffer size")
	writeBufSize  = flag.Int("write-buf-size", 1024*10, "write buffer size")
	maxPacketSize = flag.Int("max-packet-size", 1024, "max packet size")
	pendingSize   = flag.Int("pending-size", 100, "pending size")
)

type sessionHandler struct {
	sessions       *sync.Map
	beginTime      time.Time
	chStopped      chan struct{}
	pendingPackets chan gnet.Packet
}

func newSessionHandler(sessions *sync.Map, pendingSize int) *sessionHandler {
	return &sessionHandler{
		sessions:       sessions,
		beginTime:      time.Now(),
		chStopped:      make(chan struct{}),
		pendingPackets: make(chan gnet.Packet, pendingSize),
	}
}

func (s *sessionHandler) SessionPendingPacket() (gnet.Packet, bool, error) {
	select {
	case p := <-s.pendingPackets:
		if p == nil {
			return nil, false, errors.New("stopped")
		}
		return p, len(s.pendingPackets) > 0, nil
	case <-s.chStopped:
		return nil, false, errors.New("stopped")
	}
}

func (s *sessionHandler) SessionOnPacket(session *gnet.Session, packet gnet.Packet) error {
	select {
	case s.pendingPackets <- packet:
		return nil
	case <-s.chStopped:
		return errors.New("stopped")
	}
}

func (s *sessionHandler) SessionOnClosed(session *gnet.Session, err error) {
	now := time.Now()
	seconds := now.Sub(s.beginTime).Seconds()
	log.Printf("session closed: %v", err)
	log.Printf("\ttime -> %.2f(secs)", seconds)
	//log.Printf("\tpacket -> total: %d, per(s): %d, aver_size: %d (%.2fmb)", totalPacket, int64(float64(totalPacket)/seconds), averPacketSize, float64(averPacketSize)/(1024*1024))
	//log.Printf("\tbytes -> total: %d (%.2fmb) per(s): %d (%.2fmb)", totalBytes, float64(totalBytes)/(1024*1024), int64(float64(totalBytes)/seconds), float64(totalBytes)/seconds/(1024*1024))
	s.sessions.Delete(session.LocalAddr().String())
}

func main() {
	log.Default().SetFlags(log.Default().Flags() | log.Lmicroseconds)

	flag.Parse()

	listener, err := net.Listen("tcp", *serverAddr)
	if err != nil {
		log.Fatalf("listening at %v -> %v", *serverAddr, err)
	}

	sessions := &sync.Map{}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Fatalf("listening stopped, %v", err)
			}

			handler := newSessionHandler(sessions, *pendingSize)
			readWriter := define.NewPacketReaderWriter(*readBufSize, *writeBufSize, *maxPacketSize)
			s := gnet.NewSession(conn, readWriter, readWriter, handler)
			_ = s.Start()
			sessions.Store(s.LocalAddr().String(), s)
		}
	}()

	chSignal := make(chan os.Signal, 1)
	signal.Notify(chSignal, syscall.SIGINT, syscall.SIGTERM)
	<-chSignal
}
