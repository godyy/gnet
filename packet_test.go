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

type testPacket []byte

func (p testPacket) Data() []byte {
	return p
}

type testTCPPacketReaderWriter struct {
	timeout time.Duration
}

func (t testTCPPacketReaderWriter) ReadPacket(r ConnReader) (Packet, error) {
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

func (t testTCPPacketReaderWriter) WritePacket(w ConnWriter, p Packet) error {
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

func TestTCPPacket(t *testing.T) {
	addr := "localhost:8080"
	packetReaderWriter := &testTCPPacketReaderWriter{
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

			go func() {
				wg.Add(1)
				defer wg.Done()

				for {
					p, err := ReadPacket(conn, packetReaderWriter)
					if err != nil {
						log.Printf("server read packet failed, %v", err)
						return
					}

					log.Printf("server read packet: %s", p.Data())
					p = testPacket(string(p.Data()) + " received")
					if err := WritePacket(conn, p, packetReaderWriter); err != nil {
						log.Fatalf("server write packet failed, %v", err)
					}
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
	messages := []string{"hello", "what's your name?", "bye"}
	for i := 0; i < len(messages); i++ {
		p := testPacket(messages[i])
		if err := WritePacket(conn, p, packetReaderWriter); err != nil {
			log.Fatalf("client write packet failed, %v", err)
		}

		if p, err := ReadPacket(conn, packetReaderWriter); err != nil {
			log.Fatalf("client read packet failed, %v", err)
		} else {
			log.Printf("client read packet: %s", p.Data())
		}

		time.Sleep(1 * time.Second)
	}
	_ = conn.Close()

	_ = listener.Close()

	wg.Wait()
}

type testUDPPacketReaderWriter struct {
	timeout       time.Duration
	maxPacketSize int
}

func (t testUDPPacketReaderWriter) ReadPacket(r ConnReader) (Packet, error) {
	_ = r.SetReadDeadline(time.Now().Add(t.timeout))

	data := make([]byte, t.maxPacketSize)
	if n, err := r.Read(data); err != nil {
		return nil, pkg_errors.WithMessage(err, "read packet")
	} else {
		return testPacket(data[:n]), nil
	}
}

func (t testUDPPacketReaderWriter) WritePacket(w ConnWriter, p Packet) error {
	_ = w.SetWriteDeadline(time.Now().Add(t.timeout))

	if _, err := w.Write(p.Data()); err != nil {
		return pkg_errors.WithMessage(err, "write packet")
	} else {
		return nil
	}
}

func (t testUDPPacketReaderWriter) ReadPacketFrom(r ConnReaderFrom) (Packet, net.Addr, error) {
	_ = r.SetReadDeadline(time.Now().Add(t.timeout))

	data := make([]byte, t.maxPacketSize)
	if n, addr, err := r.ReadFrom(data); err != nil {
		return nil, nil, pkg_errors.WithMessage(err, "read packet from")
	} else {
		return testPacket(data[:n]), addr, nil
	}
}

func (t testUDPPacketReaderWriter) WritePacketTo(w ConnWriterTo, p Packet, addr net.Addr) error {
	_ = w.SetWriteDeadline(time.Now().Add(t.timeout))

	if _, err := w.WriteTo(p.Data(), addr); err != nil {
		return pkg_errors.WithMessage(err, "write packet to")
	} else {
		return nil
	}
}

func TestUDPPacket(t *testing.T) {
	addr := "127.0.0.1:12345"
	packetReaderWriter := &testUDPPacketReaderWriter{
		timeout:       5 * time.Second,
		maxPacketSize: 1024,
	}
	wg := &sync.WaitGroup{}

	// 监听
	listenConn, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		log.Println("start listening...")
		wg.Add(1)
		defer func() {
			log.Println("listening stopped.")
			wg.Done()
		}()

		for {
			p, addr, err := ReadPacketFrom(listenConn, packetReaderWriter)
			if err != nil {
				log.Printf("server read packet failed, %v", err)
				break
			}

			log.Printf("server read packet: %s, from:%s", p.Data(), addr)
			p = testPacket(string(p.Data()) + " received")
			if err := WritePacketTo(listenConn, p, addr, packetReaderWriter); err != nil {
				log.Fatalf("server write packet to %s failed, %v", addr, err)
			}
		}

	}()
	time.Sleep(1 * time.Second)

	// 连接
	conn, err := net.Dial("udp", addr)
	if err != nil {
		log.Fatalf("connect failed, %v", err)
	}
	messages := []string{"hello", "what's your name?", "bye"}
	for i := 0; i < len(messages); i++ {
		p := testPacket(messages[i])
		if err := WritePacket(conn, p, packetReaderWriter); err != nil {
			log.Fatalf("client write packet %s failed, %v", p, err)
		}

		if p, err := ReadPacket(conn, packetReaderWriter); err != nil {
			log.Fatalf("client read packet failed, %v", err)
		} else {
			log.Printf("client read packet: %s", p.Data())
		}

		time.Sleep(1 * time.Second)
	}
	_ = conn.Close()

	_ = listenConn.Close()

	wg.Wait()
}
