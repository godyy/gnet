package server

import (
	"errors"
	"github.com/godyy/gnet"
	"github.com/godyy/gnet/internal/examples/chat"
	"github.com/godyy/gnet/internal/examples/chat/protocol"
	"log"
	"net"
	"time"
)

type Server struct {
	opt       *gnet.TCPSessionOption
	listener  *gnet.TCPListener  // 网络监听器
	users     map[string]*user   // 用户
	requestCh chan serverRequest // 请求channel
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) getOpt() *gnet.TCPSessionOption {
	return s.opt
}

func (s *Server) Start(addr string, opt *gnet.TCPSessionOption) error {
	s.opt = opt

	var err error
	s.listener, err = gnet.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}

	s.users = map[string]*user{}
	s.requestCh = make(chan serverRequest, 100)

	go func() {
		if err := s.listener.Start(func(conn net.Conn) {
			tcpConn := conn.(*net.TCPConn)
			session := gnet.NewTCPSession(tcpConn)
			user := newUser(s, session)
			if err := user.start(); err != nil {
				log.Printf("new user star: %v", err)
				user.stop()
			}
		}); err != nil {
			log.Printf("server stop listen: %v", err)
		}
	}()

	go s.loop()

	return nil
}

func (s *Server) loop() {
	for request := range s.requestCh {
		request.process(s)
	}
}

func (s *Server) pushRequest(req serverRequest) {
	s.requestCh <- req
}

func (s *Server) broadcastNotify(p protocol.Protocol) {
	for _, u := range s.users {
		if err := u.sendMessage(&chat.Message{
			MsgType: chat.MsgTypeNotify,
			MsgSeri: 0,
			Protoc:  p,
		}); err != nil {
			log.Printf("send notify 2 user %s: %v", u.name, err)
		}
	}
}

type serverRequest interface {
	process(*Server)
}

type loginRequest struct {
	user *user
	ch   chan error
}

func newLoginRequest(user *user) *loginRequest {
	return &loginRequest{
		user: user,
		ch:   make(chan error, 1),
	}
}

func (u *loginRequest) process(s *Server) {
	if _, ok := s.users[u.user.name]; ok {
		u.ch <- errors.New("user name duplicate")
	} else {
		log.Printf("user \"%s\" enter!", u.user.name)
		s.broadcastNotify(&protocol.MessageNotify{Type: protocol.MsgTypeUserEnter, UserName: u.user.name, TimeMS: time.Now().UnixMilli()})
		s.users[u.user.name] = u.user
		u.ch <- nil
	}
	close(u.ch)
}

type logoutRequest struct {
	userName string
}

func newLogoutRequest(userName string) *logoutRequest {
	return &logoutRequest{userName: userName}
}

func (u *logoutRequest) process(s *Server) {
	if user, ok := s.users[u.userName]; ok {
		log.Printf("user \"%s\" exit!", user.name)
		delete(s.users, u.userName)
		s.broadcastNotify(&protocol.MessageNotify{Type: protocol.MsgTypeUserExit, UserName: user.name, TimeMS: time.Now().UnixMilli()})
	}
}

type messageRequest struct {
	userName string
	message  string
}

func newMessageRequest(userName, message string) *messageRequest {
	return &messageRequest{
		userName: userName,
		message:  message,
	}
}

func (m *messageRequest) process(s *Server) {
	log.Printf("user \"%s\" said: %s", m.userName, m.message)
	protoc := &protocol.MessageNotify{
		Type:     protocol.MsgTypeUserMessage,
		UserName: m.userName,
		Message:  m.message,
		TimeMS:   time.Now().UnixMilli(),
	}
	s.broadcastNotify(protoc)
}

type notifyRequest struct {
	protoc protocol.Protocol
}
