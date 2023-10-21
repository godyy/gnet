package gnet

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
)

type testSessionHandler struct {
	onSessionPacket func(Session, *Packet) error
	onSessionClose  func(Session, error)
}

func (h *testSessionHandler) OnSessionPacket(session Session, packet *Packet) error {
	return h.onSessionPacket(session, packet)
}

func (h *testSessionHandler) OnSessionClosed(session Session, err error) {
	h.onSessionClose(session, err)
}

func TestTCP(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	opt := NewTCPSessionOption().
		//SetSendTimeout(10 * time.Second).
		//SetReceiveTimeout(10 * time.Second).
		SetMaxPacketSize(128).
		SetSendBufferSize(5).
		SetSendBufferMaxSize(50).
		SetReceiveBufferSize(5)
	addr := "localhost:9999"

	serverSessionCount := &sync.WaitGroup{}
	serverHandler := &testSessionHandler{
		onSessionPacket: func(session Session, packet *Packet) error {
			defer PutPacket(packet)
			size := packet.Readable()
			content, _ := packet.ReadString()
			t.Logf("receive packet, size: %d content:%s", size, content)
			//session.Close("shutdown")
			return nil
		},
		onSessionClose: func(session Session, err error) {
			t.Logf("server session close: %v", err)
			serverSessionCount.Done()
		},
	}

	listener, err := ListenTCP("tcp", "localhost:9999")
	if err != nil {
		t.Fatal(err)
	}

	chListenerStopped := make(chan error, 1)
	go func() {
		err := listener.Start(func(conn net.Conn) {
			tcpConn := conn.(*net.TCPConn)
			serverSessionCount.Add(1)
			session := NewTCPSession(tcpConn)
			if err := session.Start(opt, serverHandler); err != nil {
				t.Log("session start", err)
			}
		})
		chListenerStopped <- err
		close(chListenerStopped)
	}()

	clientSessionCount := 10
	clientSessionCountWait := &sync.WaitGroup{}
	clientHandler := &testSessionHandler{
		onSessionPacket: func(session Session, packet *Packet) error { return nil },
		onSessionClose: func(session Session, err error) {
			t.Logf("client session close: %v", err)
			clientSessionCountWait.Done()
		},
	}
	for i := 0; i < clientSessionCount; i++ {
		session, err := ConnectTCP("tcp", addr)
		if err != nil {
			t.Fatal("connect:", err)
		}
		clientSessionCountWait.Add(1)
		if err := session.Start(opt, clientHandler); err != nil {
			t.Fatal("session start:", err)
		}
		go func(session *TCPSession, no int) {
			for i := 0; i < 10; i++ {
				p := GetPacket()
				p.WriteString(fmt.Sprintf("message %d:%d", no, i))
				if err := session.SendPacket(p); err != nil {
					fmt.Println("send:", err)
				}
				time.Sleep(time.Duration(rand.Int63n(10 * int64(time.Millisecond))))
			}
			time.Sleep(1 * time.Second)
			session.Close(errors.New("shutdown"))
		}(session, i)
	}

	_ = listener.Close()
	if err := <-chListenerStopped; err != nil {
		t.Log("listener close:", err)
	}

	clientSessionCountWait.Wait()
	serverSessionCount.Wait()

	if !opt.lock() {
		t.Fatal("option ref-count > 0")
	}
}

func TestTCPPacketGreaterThanBuffer(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	opt := NewTCPSessionOption().
		SetMaxPacketSize(10240)
	addr := "localhost:9999"

	serverSessionCount := &sync.WaitGroup{}
	serverHandler := &testSessionHandler{
		onSessionPacket: func(session Session, packet *Packet) error {
			defer PutPacket(packet)
			size := packet.Readable()
			content := string(packet.UnreadData())
			t.Logf("receive packet, size: %d content:%s", size, content)
			return nil
		},
		onSessionClose: func(session Session, err error) {
			serverSessionCount.Done()
			t.Logf("session close: %v", err)
		},
	}

	listener, err := ListenTCP("tcp", "localhost:9999")
	if err != nil {
		t.Fatal(err)
	}

	chListenerStopped := make(chan error, 1)
	go func() {
		err := listener.Start(func(conn net.Conn) {
			tcpConn := conn.(*net.TCPConn)
			serverSessionCount.Add(1)
			session := NewTCPSession(tcpConn)
			if err := session.Start(opt, serverHandler); err != nil {
				t.Log("session start", err)
			}
		})
		chListenerStopped <- err
		close(chListenerStopped)
	}()

	clientHandler := &testSessionHandler{
		onSessionPacket: func(session Session, packet *Packet) error { return nil },
		onSessionClose:  func(session Session, err error) {},
	}
	session, err := ConnectTCP("tcp", addr)
	if err != nil {
		t.Fatal("connect:", err)
	}

	if err := session.Start(opt, clientHandler); err != nil {
		t.Fatal("session start:", err)
	}

	for i := 0; i < 10; i++ {
		data := make([]byte, 10240-rand.Intn(10)*128)
		for j := 0; j < len(data); j++ {
			data[j] = 48 + byte(i)
		}
		p := NewPacketWithData(data)
		t.Logf("send %d", p.Readable())
		if err := session.SendPacket(p); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second)
	}

	session.Close(errors.New("shutdown"))

	_ = listener.Close()
	if err := <-chListenerStopped; err != nil {
		t.Log("listener close:", err)
	}

	serverSessionCount.Wait()
}

func TestTCPExceedMaxPacket(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	opt := NewTCPSessionOption().
		SetMaxPacketSize(128)
	addr := "localhost:9999"

	serverSessionCount := &sync.WaitGroup{}
	serverHandler := &testSessionHandler{
		onSessionPacket: func(session Session, packet *Packet) error {
			defer PutPacket(packet)
			size := packet.Readable()
			content := string(packet.UnreadData())
			t.Logf("receive packet, size: %d content:%s", size, content)
			return nil
		},
		onSessionClose: func(session Session, err error) {
			t.Logf("session close: %v", err)
			serverSessionCount.Done()
		},
	}

	listener, err := ListenTCP("tcp", "localhost:9999")
	if err != nil {
		t.Fatal(err)
	}

	chListenerStopped := make(chan error, 1)
	go func() {
		err := listener.Start(func(conn net.Conn) {
			tcpConn := conn.(*net.TCPConn)
			serverSessionCount.Add(1)
			session := NewTCPSession(tcpConn)
			if err := session.Start(opt, serverHandler); err != nil {
				t.Log("session start", err)
			}
		})
		chListenerStopped <- err
		close(chListenerStopped)
	}()

	optCopy := *opt
	optCopy.SetMaxPacketSize(256)
	clientHandler := &testSessionHandler{
		onSessionPacket: func(session Session, packet *Packet) error { return nil },
		onSessionClose:  func(session Session, err error) {},
	}
	session, err := ConnectTCP("tcp", addr)
	if err != nil {
		t.Fatal("connect:", err)
	}
	if err := session.Start(&optCopy, clientHandler); err != nil {
		t.Fatal("session start:", err)
	}
	if err := session.SendPacket(NewPacketWithData(make([]byte, 256))); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	serverSessionCount.Wait()
	session.Close(errors.New("shutdown"))
	_ = listener.Close()
	if err := <-chListenerStopped; err != nil {
		t.Log("listener close:", err)
	}
}
