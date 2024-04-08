package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/godyy/gnet"
)

var packetPool = &sync.Pool{
	New: func() any {
		return gnet.NewPacket(nil)
	},
}

type sessionHandler struct {
	sessions      *sync.Map
	beginTime     time.Time
	packetCounter atomic.Int64
	bytesCounter  atomic.Int64
}

func newSessionHandler(sessions *sync.Map) *sessionHandler {
	return &sessionHandler{
		sessions:  sessions,
		beginTime: time.Now(),
	}
}

func (s *sessionHandler) GetPacket(size int) gnet.CustomPacket {
	p := packetPool.Get().(*gnet.Packet)
	p.Grow(size, true)
	return p
}

func (s *sessionHandler) PutPacket(p gnet.CustomPacket) {
	p.(*gnet.Packet).Reset()
	packetPool.Put(p)
}

func (s *sessionHandler) OnSessionPacket(session gnet.Session, packet gnet.CustomPacket) error {
	s.packetCounter.Add(1)
	s.bytesCounter.Add(int64(len(packet.Data())) + 4)
	return session.SendPacket(packet)
}

func (s *sessionHandler) OnSessionClosed(session gnet.Session, err error) {
	now := time.Now()
	seconds := now.Sub(s.beginTime).Seconds()
	totalPacket := s.packetCounter.Load()
	totalBytes := s.bytesCounter.Load()
	averPacketSize := int64(float64(totalBytes) / float64(totalPacket))
	log.Printf("session closed: %v", err)
	log.Printf("\ttime -> %.2f(secs)", seconds)
	log.Printf("\tpacket -> total: %d, per(s): %d, aver_size: %d (%.2fmb)", totalPacket, int64(float64(totalPacket)/seconds), averPacketSize, float64(averPacketSize)/(1024*1024))
	log.Printf("\tbytes -> total: %d (%.2fmb) per(s): %d (%.2fmb)", totalBytes, float64(totalBytes)/(1024*1024), int64(float64(totalBytes)/seconds), float64(totalBytes)/seconds/(1024*1024))
	s.sessions.Delete(session.LocalAddr().String())
}

func main() {
	log.Default().SetFlags(log.Default().Flags() | log.Lmicroseconds)

	serverAddr := flag.String("server-addr", ":2222", "server address")
	sendBufferSize := flag.Int("send-buffer-size", 1024*10, "send buffer size")
	receiveBufferSize := flag.Int("receive-buffer-size", 1024*10, "receive buffer size")
	maxPacketSize := flag.Int("max-packet-size", 1024, "max packet size")
	flag.Parse()

	sessionCfg := gnet.NewTcpSessionCfg()
	sessionCfg.SendBufferSize = *sendBufferSize
	sessionCfg.ReceiveBufferSize = *receiveBufferSize
	sessionCfg.MaxPacketSize = *maxPacketSize

	log.Printf("%+v", sessionCfg)

	listener, err := gnet.ListenTCP("tcp", *serverAddr)
	if err != nil {
		log.Fatalf("listening at %v -> %v", *serverAddr, err)
	}

	sessions := &sync.Map{}

	go func() {
		err := listener.Start(func(conn *net.TCPConn) {
			session := gnet.NewTCPSession(conn, sessionCfg)
			if err := session.Start(newSessionHandler(sessions)); err != nil {
				log.Fatalf("session start -> %v", err)
			} else {
				log.Println("session started.")
			}
			sessions.Store(session.LocalAddr().String(), session)
		})
		if err != nil {
			log.Printf("listening stopped -> %v", err)
		}
	}()

	chSignal := make(chan os.Signal, 1)
	signal.Notify(chSignal, syscall.SIGINT, syscall.SIGTERM)
	<-chSignal
}
