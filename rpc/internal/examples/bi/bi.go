package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/godyy/gnet"
	"github.com/godyy/gnet/rpc"
	pkg_errors "github.com/pkg/errors"
	"log"
	"net"
	"sync"
	"time"
)

const (
	readWriteTimeout = 10 * time.Second
)

type packetReadWriter struct{}

func (rw *packetReadWriter) ReadPacket(r gnet.ConnReader) (gnet.Packet, error) {
	_ = r.SetReadDeadline(time.Now().Add(readWriteTimeout))

	var head [4]byte
	if _, err := r.Read(head[:]); err != nil {
		return nil, pkg_errors.WithMessage(err, "read head")
	}
	size := binary.BigEndian.Uint32(head[:])

	data := make([]byte, size)
	if _, err := r.Read(data); err != nil {
		return nil, pkg_errors.WithMessage(err, "read body")
	}

	return gnet.RawPacket(data), nil
}

func (rw *packetReadWriter) WritePacket(w gnet.ConnWriter, p gnet.Packet) error {
	_ = w.SetWriteDeadline(time.Now().Add(readWriteTimeout))

	var head [4]byte
	binary.BigEndian.PutUint32(head[:], uint32(len(p.Data())))
	if _, err := w.Write(head[:]); err != nil {
		return pkg_errors.WithMessage(err, "write head")
	}

	if _, err := w.Write(p.Data()); err != nil {
		return pkg_errors.WithMessage(err, "write body")
	}

	return nil
}

type conn struct {
	net.Conn
	sync.Mutex
}

func (c *conn) WritePacket(p gnet.Packet) error {
	c.Lock()
	defer c.Unlock()
	return gnet.WritePacket(c.Conn, p, _packetReadWriter)
}

type echoArgs struct {
	Msg string `json:"msg"`
}

type echoReply struct {
	Msg string `json:"msg"`
}

type clientHandler struct{}

func (c *clientHandler) RPCEncodeMethodArgs(methodId uint16, args any) ([]byte, error) {
	if methodId != 0 {
		return nil, errors.New("method not exist")
	}

	if _, ok := args.(*echoArgs); !ok {
		return nil, errors.New("wrong type of method arguments")
	}

	argsBytes, err := json.Marshal(args)
	if err != nil {
		return nil, pkg_errors.WithMessage(err, "marshal args")
	}

	return argsBytes, nil
}

func (c *clientHandler) RPCDecodeMethodReply(methodId uint16, replyBytes []byte) (any, error) {
	if methodId != 0 {
		return nil, errors.New("method not exist")
	}

	rsp := &echoReply{}
	if err := json.Unmarshal(replyBytes, &rsp); err != nil {
		return nil, pkg_errors.WithMessage(err, "unmarshal reply")
	}

	return rsp, nil
}

func (c *clientHandler) RPCEncodeRequestPacket(size int, encoder func(p []byte) error) (gnet.Packet, error) {
	p := make([]byte, 1+size)
	p[0] = 1

	if err := encoder(p[1:]); err != nil {
		return nil, err
	}

	return gnet.RawPacket(p), nil
}

type serverHandler struct{}

func (s *serverHandler) RPCDecodeMethodArgs(methodId uint16, argsBytes []byte) (any, error) {
	if methodId != 0 {
		return nil, errors.New("method not exist")
	}

	rsp := &echoArgs{}
	if err := json.Unmarshal(argsBytes, &rsp); err != nil {
		return nil, pkg_errors.WithMessage(err, "unmarshal args")
	}

	return rsp, nil
}

func (s *serverHandler) RPCEncodeMethodReply(methodId uint16, reply any) ([]byte, error) {
	if methodId != 0 {
		return nil, errors.New("method not exist")
	}

	if _, ok := reply.(*echoReply); !ok {
		return nil, errors.New("wrong type of method reply")
	}

	argsBytes, err := json.Marshal(reply)
	if err != nil {
		return nil, pkg_errors.WithMessage(err, "marshal reply")
	}

	return argsBytes, nil
}

func (s *serverHandler) RPCEncodeResponsePacket(size int, encoder func(p []byte) error) (gnet.Packet, error) {
	p := make([]byte, 1+size)
	p[0] = 2

	if err := encoder(p[1:]); err != nil {
		return nil, err
	}

	return gnet.RawPacket(p), nil
}

func (s *serverHandler) RPCHandleRequest(req *rpc.Request) error {
	reply, err := methodEcho.OnCall(req.Args)
	if err != nil {
		return req.Error(err)
	}
	return req.Reply(reply)
}

var (
	_packetReadWriter = &packetReadWriter{}
	cClient, cServer  *rpc.Client
	server            *rpc.Server
	methodEcho        = rpc.NewMethodG(0, func(args *echoArgs) (*echoReply, error) {
		return &echoReply{Msg: args.Msg}, nil
	})
	addr = ":2345"
	n    = 1000
	wg   sync.WaitGroup
)

func main() {
	clientHandler := &clientHandler{}
	serverHandler := &serverHandler{}
	server = rpc.NewServer(serverHandler)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen failed, %v", err)
	}

	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				log.Fatalf("accept failed, %v", err)
			}

			go func(c net.Conn) {
				defer c.Close()
				conn := &conn{Conn: c}
				cServer = rpc.NewClient(conn, clientHandler)
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < n; i++ {
						begin := time.Now()
						args := &echoArgs{Msg: fmt.Sprintf("echo server to client %d", i)}
						reply, err := methodEcho.Call(cServer, args, 100)
						if err != nil {
							log.Fatalf("server call client %d, %v", i, err)
						}
						log.Printf("server call client %d, response:%s, used:%d", i, reply.Msg, time.Now().Sub(begin).Milliseconds())
					}
				}()
				for {
					p, err := gnet.ReadPacket(conn, _packetReadWriter)
					if err != nil {
						log.Printf("server read packet failed, %v", err)
						return
					}

					switch p.Data()[0] {
					case 1:
						if err := server.HandleRequest(conn, p.Data()[1:]); err != nil {
							log.Printf("server handle request failed, %v", err)
						}

					case 2:
						if err := cServer.HandleResponse(p.Data()[1:]); err != nil {
							log.Printf("server handle response failed, %v", err)
						}

					default:
						log.Println("server read invalid packet")
					}
				}
			}(c)
		}
	}()

	c, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("dial failed, %v", err)
	}
	conn := &conn{Conn: c}
	cClient = rpc.NewClient(conn, clientHandler)

	go func() {
		defer c.Close()
		for {
			p, err := gnet.ReadPacket(conn, _packetReadWriter)
			if err != nil {
				log.Printf("client read packet failed, %v", err)
				return
			}

			switch p.Data()[0] {
			case 1:
				if err := server.HandleRequest(conn, p.Data()[1:]); err != nil {
					log.Printf("client handle request failed, %v", err)
				}

			case 2:
				if err := cClient.HandleResponse(p.Data()[1:]); err != nil {
					log.Printf("client handle response failed, %v", err)
				}

			default:
				log.Println("client read invalid packet")
			}
		}
	}()

	for i := 0; i < n; i++ {
		begin := time.Now()
		args := &echoArgs{Msg: fmt.Sprintf("echo client to server %d", i)}
		reply, err := methodEcho.Call(cClient, args, 100)
		if err != nil {
			log.Fatalf("client call server %d, %v", i, err)
		}
		log.Printf("client call server %d, response:%s, used:%d", i, reply.Msg, time.Now().Sub(begin).Milliseconds())
	}

	wg.Wait()
}
