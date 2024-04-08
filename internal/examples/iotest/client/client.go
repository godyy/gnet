package main

import (
	"encoding/binary"
	"flag"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/godyy/gnet"
)

var packetPool = &sync.Pool{
	New: func() any {
		return gnet.NewPacket(nil)
	},
}

type sessionHandler struct {
	packetCounter atomic.Int64
	bytesCounter  atomic.Int64
	rttCounter    atomic.Int64
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

func (s *sessionHandler) OnSessionPacket(session gnet.Session, cp gnet.CustomPacket) error {
	s.packetCounter.Add(1)
	s.bytesCounter.Add(int64(len(cp.Data())) + 4)
	p := cp.(*gnet.Packet)
	sendTimeNano, err := p.ReadInt64()
	if err != nil {
		return errors.WithMessage(err, "read send time")
	}
	rttNano := time.Now().UnixNano() - sendTimeNano
	s.rttCounter.Add(rttNano)
	p.Reset()
	packetPool.Put(p)
	return nil
}

func (s *sessionHandler) OnSessionClosed(session gnet.Session, err error) {
	log.Println("session closed: ", err)
}

func main() {
	log.Default().SetFlags(log.Default().Flags() | log.Lmicroseconds)

	serverAddr := flag.String("server-addr", ":2222", "server address")
	sendBufferSize := flag.Int("send-buffer-size", 1024*10, "send buffer size")
	receiveBufferSize := flag.Int("receive-buffer-size", 1024*10, "receive buffer size")
	minPacketSize := flag.Int("min-packet-size", 1024, "min pakcet size")
	maxPacketSize := flag.Int("max-packet-size", 1024, "max packet size")
	concurrentCount := flag.Int("concurrent-count", 100, "concurrent-count")
	testTime := flag.Int("test-time", 5000, "test time in milliseconds")
	flag.Parse()

	sessionCfg := gnet.NewTcpSessionCfg()
	sessionCfg.SendBufferSize = *sendBufferSize
	sessionCfg.ReceiveBufferSize = *receiveBufferSize
	sessionCfg.MaxPacketSize = *maxPacketSize
	handler := &sessionHandler{}

	log.Printf("%+v", sessionCfg)

	conn, err := gnet.ConnectTCP("tcp", *serverAddr)
	if err != nil {
		log.Fatalf("connect server %v -> %v", *serverAddr, err)
	}

	beginTime := time.Now()
	rand.Seed(beginTime.UnixNano())

	clientSession := gnet.NewTCPSession(conn, sessionCfg)
	if err = clientSession.Start(handler); err != nil {
		log.Fatalf("start session -> %v", err)
	}

	for i := 0; i < *concurrentCount; i++ {
		go func() {
			for {
				size := *minPacketSize + rand.Intn(*maxPacketSize-*minPacketSize+1)
				p := packetPool.Get().(*gnet.Packet)
				p.Grow(size, true)
				binary.BigEndian.PutUint64(p.Data(), uint64(time.Now().UnixNano()))
				if err := clientSession.SendPacket(p); err != nil {
					break
				}
				ms := rand.Int63n(500) + 500
				time.Sleep(time.Duration(ms) * time.Millisecond)
				//time.Sleep(time.Duration(rand.Int63n(5)+1) * time.Millisecond)
			}
		}()
	}

	time.Sleep(time.Duration(*testTime) * time.Millisecond)
	_ = clientSession.Close(errors.New("stop"))

	now := time.Now()
	seconds := now.Sub(beginTime).Seconds()
	totalPacket := handler.packetCounter.Load()
	totalBytes := handler.bytesCounter.Load()
	totalRtt := handler.rttCounter.Load()
	averPacketSize := int64(float64(totalBytes) / float64(totalPacket))
	log.Printf("time -> %.2fs", seconds)
	log.Printf("packet -> total: %d, per(s): %d, aver_size: %d (%.4fmb), aver_rtt: %.2fms", totalPacket, int64(float64(totalPacket)/seconds), averPacketSize, float64(averPacketSize)/(1024*1024), float64(totalRtt)/float64(totalPacket)/float64(time.Millisecond))
	log.Printf("bytes -> total: %d (%.2fmb) per(s): %d (%.2fmb)", totalBytes, float64(totalBytes)/(1024*1024), int64(float64(totalBytes)/seconds), float64(totalBytes)/seconds/(1024*1024))
}
