package main

import (
	"encoding/binary"
	"flag"
	"github.com/godyy/gnet/internal/examples/iotest/define"
	"log"
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/godyy/gnet"
)

type statistics struct {
	packetCounter atomic.Int64
	bytesCounter  atomic.Int64
	rttCounter    atomic.Int64
}

type sessionHandler struct {
	*statistics
	chStopped      chan struct{}
	pendingPackets chan gnet.Packet
}

func newSessionHandler(pendingSize int, statistics *statistics) *sessionHandler {
	return &sessionHandler{
		statistics:     statistics,
		pendingPackets: make(chan gnet.Packet, pendingSize),
		chStopped:      make(chan struct{}),
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

func (s *sessionHandler) SessionOnPacket(session *gnet.Session, p gnet.Packet) error {
	s.statistics.packetCounter.Add(1)
	s.statistics.bytesCounter.Add(int64(len(p.Data())) + 4)
	pp := p.(*gnet.Buffer)
	sendTimeNano, err := pp.ReadInt64()
	if err != nil {
		return errors.WithMessage(err, "read send time")
	}
	rttNano := time.Now().UnixNano() - sendTimeNano
	s.statistics.rttCounter.Add(rttNano)
	define.PutBuffer(pp)
	return nil
}

func (s *sessionHandler) SessionOnClosed(session *gnet.Session, err error) {
	log.Println("session closed: ", err)
}

func (s *sessionHandler) SendPacket(p gnet.Packet) error {
	select {
	case s.pendingPackets <- p:
		return nil
	case <-s.chStopped:
		return errors.New("stopped")
	}
}

func (s *sessionHandler) Stop() {
	close(s.chStopped)
}

func main() {
	log.Default().SetFlags(log.Default().Flags() | log.Lmicroseconds)

	serverAddr := flag.String("server-addr", ":2222", "server address")
	readBufSize := flag.Int("read-buf-size", 1024*10, "read buffer size")
	writeBufSize := flag.Int("write-buf-size", 1024*10, "write buffer size")
	minPacketSize := flag.Int("min-packet-size", 1024, "min pakcet size")
	maxPacketSize := flag.Int("max-packet-size", 1024, "max packet size")
	pendingSize := flag.Int("pending-size", 100, "pending size")
	connections := flag.Int("connections", 100, "connection count")
	testTime := flag.Int("test-time", 5000, "test time in milliseconds")
	flag.Parse()

	beginTime := time.Now()
	rand.Seed(beginTime.UnixNano())

	statistics := &statistics{}

	// 创建连接
	sessions := make([]*gnet.Session, *connections)
	handlers := make([]*sessionHandler, *connections)
	for i := 0; i < *connections; i++ {
		go func(i int) {
			conn, err := net.Dial("tcp", *serverAddr)
			if err != nil {
				log.Fatalf("connect failed, %v", err)
			}
			handler := newSessionHandler(*pendingSize, statistics)
			readWriter := define.NewPacketReaderWriter(*readBufSize, *writeBufSize, *maxPacketSize)
			session := gnet.NewSession(conn, readWriter, readWriter, handler)
			_ = session.Start()
			sessions[i] = session
			handlers[i] = handler

			for {
				size := *minPacketSize + rand.Intn(*maxPacketSize-*minPacketSize+1)
				p := define.GetBuffer()
				p.Grow(size, true)
				binary.BigEndian.PutUint64(p.Data(), uint64(time.Now().UnixNano()))
				if err := handler.SendPacket(p); err != nil {
					break
				}
				//time.Sleep(1 * time.Millisecond)
				//ms := rand.Int63n(500) + 500
				//time.Sleep(time.Duration(ms) * time.Millisecond)
				//time.Sleep(time.Duration(rand.Int63n(5)+1) * time.Millisecond)
			}
		}(i)
	}

	time.Sleep(time.Duration(*testTime) * time.Millisecond)
	for i := 0; i < *connections; i++ {
		_ = sessions[i].Close()
		handlers[i].Stop()
	}

	now := time.Now()
	seconds := now.Sub(beginTime).Seconds()
	totalPacket := statistics.packetCounter.Load()
	totalBytes := statistics.bytesCounter.Load()
	totalRtt := statistics.rttCounter.Load()
	averPacketSize := int64(float64(totalBytes) / float64(totalPacket))
	log.Printf("time -> %.2fs", seconds)
	log.Printf("packet -> total: %d, per(s): %d, aver_size: %d (%.4fmb), aver_rtt: %.2fms", totalPacket, int64(float64(totalPacket)/seconds), averPacketSize, float64(averPacketSize)/(1024*1024), float64(totalRtt)/float64(totalPacket)/float64(time.Millisecond))
	log.Printf("bytes -> total: %d (%.2fmb) per(s): %d (%.2fmb)", totalBytes, float64(totalBytes)/(1024*1024), int64(float64(totalBytes)/seconds), float64(totalBytes)/seconds/(1024*1024))
}
