package gnet

import (
	"encoding/binary"
	"errors"
	pkg_errors "github.com/pkg/errors"
	"log"
	"math"
	"net"
	"sync"
	"testing"
	"time"
)

type tcpSessionPacket []byte

func (p tcpSessionPacket) Data() []byte {
	return p
}

type tcpSessionPacketReaderWriter struct {
	timeout time.Duration
}

func (t *tcpSessionPacketReaderWriter) SessionReadPacket(r ConnReader) (Packet, error) {
	_ = r.SetReadDeadline(time.Now().Add(t.timeout))

	var head [2]byte
	_, err := r.Read(head[:])
	if err != nil {
		return nil, pkg_errors.WithMessage(err, "read head")
	}

	n := int(binary.BigEndian.Uint16(head[:]))
	body := make([]byte, n)
	if _, err := r.Read(body); err != nil {
		return nil, pkg_errors.WithMessage(err, "read body")
	}

	return testPacket(body), nil
}

func (t *tcpSessionPacketReaderWriter) SessionWritePacket(w ConnWriter, p Packet, more bool) error {
	n := len(p.Data())
	if n > math.MaxUint16 {
		return errors.New("body length overflow")
	}

	_ = w.SetWriteDeadline(time.Now().Add(t.timeout))

	var head [2]byte
	binary.BigEndian.PutUint16(head[:], uint16(n))
	if _, err := w.Write(head[:]); err != nil {
		return pkg_errors.WithMessage(err, "write head")
	}

	if _, err := w.Write(p.Data()); err != nil {
		return pkg_errors.WithMessage(err, "write body")
	}
	return nil
}

type tcpSessionHandler struct {
	name           string
	pendingPackets chan Packet
	readPackets    chan Packet
}

func (t *tcpSessionHandler) Close() {
	close(t.pendingPackets)
}

func (t *tcpSessionHandler) ReadPacket() Packet {
	return <-t.readPackets
}

func (t *tcpSessionHandler) WritePacket(p Packet) {
	t.pendingPackets <- p
}

func (t *tcpSessionHandler) SessionPendingPacket() (p Packet, more bool, err error) {
	p = <-t.pendingPackets
	if p == nil {
		return nil, false, errors.New("tcp session handler closed")
	}
	return p, len(t.pendingPackets) > 0, nil
}

func (t *tcpSessionHandler) SessionOnPacket(s *Session, p Packet) error {
	t.readPackets <- p
	return nil
}

func (t *tcpSessionHandler) SessionOnClosed(s *Session, err error) {
	log.Printf("%s closed: %v", t.name, err)
	close(t.readPackets)
}

func TestTCPSession(t *testing.T) {
	addr := "localhost:8080"
	packetReaderWriter := &tcpSessionPacketReaderWriter{
		timeout: 5 * time.Second,
	}
	wg := &sync.WaitGroup{}

	// 监听
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		log.Println("start listening...")
		wg.Add(1)
		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					log.Println("listening stopped.")
					wg.Done()
					return
				}
				log.Fatal(err)
			}

			wg.Add(1)
			handler := &tcpSessionHandler{
				name:           "server",
				pendingPackets: make(chan Packet, 10),
				readPackets:    make(chan Packet, 10),
			}
			_ = NewSession(conn, packetReaderWriter, packetReaderWriter, handler).Start()
			go func() {
				defer wg.Done()
				for {
					p := handler.ReadPacket()
					if p == nil {
						return
					}
					log.Printf("server read packet: %s", p.Data())
					p = tcpSessionPacket(string(p.Data()) + " received")
					handler.WritePacket(p)
				}
			}()
		}
	}()
	time.Sleep(1 * time.Second)

	// 连接
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("connect failed, %v", err)
	}
	client := &tcpSessionHandler{
		name:           "client",
		pendingPackets: make(chan Packet, 10),
		readPackets:    make(chan Packet, 10),
	}
	s := NewSession(conn, packetReaderWriter, packetReaderWriter, client)
	_ = s.Start()
	messages := []string{"hello", "what's your name?", "bye"}
	for i := 0; i < len(messages); i++ {
		p := tcpSessionPacket(messages[i])

		client.WritePacket(p)
		if p := client.ReadPacket(); p == nil {
			break
		} else {
			log.Printf("client read packet: %s", p.Data())
		}

		time.Sleep(1 * time.Second)
	}

	client.Close()

	_ = listener.Close()

	wg.Wait()
}
