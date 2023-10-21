package main

import (
	"encoding/binary"
	"flag"
	"log"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/godyy/gnet"
)

type sessionHandler struct {
	packetCounter atomic.Int64
	bytesCounter  atomic.Int64
	rttCounter    atomic.Int64
}

func (s *sessionHandler) OnSessionPacket(session gnet.Session, packet *gnet.Packet) error {
	s.packetCounter.Add(1)
	s.bytesCounter.Add(int64(packet.Readable()))
	sendTimeNano, err := packet.ReadInt64()
	if err != nil {
		return errors.WithMessage(err, "read send time")
	}
	rttNano := time.Now().UnixNano() - sendTimeNano
	s.rttCounter.Add(rttNano)
	//log.Println(rttNano)
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
	maxPacketSize := flag.Int("max-packet-size", 1024, "max packet size")
	concurrentCount := flag.Int("concurrent-count", 100, "concurrent-count")
	testTime := flag.Int("test-time", 5000, "test time in milliseconds")
	flag.Parse()

	sessionOpt := gnet.NewTCPSessionOption().
		SetSendBufferSize(*sendBufferSize).
		SetReceiveBufferSize(*receiveBufferSize).
		SetMaxPacketSize(*maxPacketSize)
	handler := &sessionHandler{}

	log.Printf("%+v", sessionOpt)

	clientSession, err := gnet.ConnectTCP("tcp", *serverAddr)
	if err != nil {
		log.Fatalf("connect server %v -> %v", *serverAddr, err)
	}

	beginTime := time.Now()
	rand.Seed(beginTime.UnixNano())

	if err = clientSession.Start(sessionOpt, handler); err != nil {
		log.Fatalf("start session -> %v", err)
	}

	for i := 0; i < *concurrentCount; i++ {
		go func() {
			for {
				bytes := make([]byte, *maxPacketSize)
				binary.BigEndian.PutUint64(bytes, uint64(time.Now().UnixNano()))
				if err := clientSession.SendPacket(gnet.NewPacketWithData(bytes)); err != nil {
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
