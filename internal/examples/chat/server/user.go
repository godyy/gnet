package server

import (
	"fmt"
	"github.com/godyy/gnet"
	"github.com/godyy/gnet/internal/examples/chat"
	"github.com/godyy/gnet/internal/examples/chat/protocol"
	"github.com/pkg/errors"
	"log"
	"sync/atomic"
	"unicode/utf8"
)

const (
	userStarted = 1
	userStopped = 2
)

type user struct {
	server  *Server          // 所属服务
	session *gnet.TCPSession // 网络会话
	state   int32            // 状态
	name    string           // 用户名
}

func newUser(server *Server, session *gnet.TCPSession) *user {
	u := &user{
		server:  server,
		session: session,
	}
	return u
}

func (u *user) start() error {
	if atomic.CompareAndSwapInt32(&u.state, 0, userStarted) {
		if err := u.session.Start(u.server.getOpt(), u); err != nil {
			return err
		}
		return nil
	} else {
		return errors.New("already started")
	}
}

func (u *user) stop() {
	if atomic.CompareAndSwapInt32(&u.state, userStarted, userStopped) {
		if u.server != nil && u.name != "" {
			u.server.pushRequest(newLogoutRequest(u.name))
		}

		_ = u.session.Close(errors.New(""))
		u.server = nil
		u.session = nil
	}
}

func (u *user) auth(msg *chat.Message) (err error) {
	if msg.MsgType != chat.MsgTypeRequest {
		return errors.New("auth msg-type not request")
	}

	if msg.MsgSeri != 1 {
		return errors.New("auth msg-seri not 1")
	}

	loginReq, ok := msg.Protoc.(*protocol.LoginReq)
	if !ok {
		return errors.New("auth protocol not LoginReq")
	}
	loginRsp := &protocol.LoginRsp{}
	if loginReq.UserName == "" {
		loginRsp.Error = "user name empty"
		err = errors.New("user name empty")
	}
	if utf8.RuneCountInString(loginReq.UserName) > chat.MaxLengthOfUserName {
		loginRsp.Error = "length of user name exceed limit"
		err = errors.New("length of user name exceed limit")
	}
	if err != nil {
		_ = u.sendMessage(&chat.Message{
			MsgType: chat.MsgTypeResponse,
			MsgSeri: msg.MsgSeri,
			Protoc:  loginRsp,
		})
		return err
	}
	u.name = loginReq.UserName
	req := newLoginRequest(u)
	u.server.pushRequest(req)
	err = <-req.ch
	if err != nil {
		loginRsp.Error = err.Error()
	}
	_ = u.sendMessage(&chat.Message{
		MsgType: chat.MsgTypeResponse,
		MsgSeri: msg.MsgSeri,
		Protoc:  loginRsp,
	})
	return err
}

func (u *user) handleMessage(msg *chat.Message) error {
	if u.name == "" {
		return u.auth(msg)
	}

	switch msg.MsgType {
	case chat.MsgTypeNotify:
		return u.handleNotify(msg)
	case chat.MsgTypeRequest:
		return u.handleRequest(msg)
	default:
		return fmt.Errorf("invalid message type %d", msg.MsgType)
	}
}

func (u *user) handleRequest(msg *chat.Message) (err error) {
	var rspProtoc protocol.Protocol

	switch protoc := msg.Protoc.(type) {
	case *protocol.SendMessageReq:
		// 检查消息内容
		if len(protoc.Message) > chat.MaxLengthOfChatContent {
			rspProtoc = &protocol.SendMessageRsp{Error: "length of content exceed limit"}
			break
		}

		// 广播消息
		u.server.pushRequest(newMessageRequest(u.name, protoc.Message))

		rspProtoc = &protocol.SendMessageRsp{}
	default:
		return errors.New("invalid request")
	}

	return u.sendMessage(&chat.Message{
		MsgType: chat.MsgTypeResponse,
		MsgSeri: msg.MsgSeri,
		Protoc:  rspProtoc,
	})
}

func (u *user) handleNotify(msg *chat.Message) error {
	switch protoc := msg.Protoc.(type) {
	case *protocol.HeartbeatReq:
		return u.sendMessage(&chat.Message{
			MsgType: chat.MsgTypeNotify,
			MsgSeri: 0,
			Protoc:  protoc,
		})
	}

	return nil
}

func (u *user) sendMessage(msg *chat.Message) error {
	packet, err := chat.EncodeMessage(msg)
	if err != nil {
		return errors.WithMessage(err, "encode message")
	}
	return u.session.SendPacket(packet)
}

func (u *user) logf(f string, v ...interface{}) {
	log.Printf(fmt.Sprintf("user \"%s\": %s", u.name, f), v...)
}

func (u *user) OnSessionPacket(session gnet.Session, packet *gnet.Packet) error {
	defer gnet.PutPacket(packet)

	msg, err := chat.DecodeMessage(packet)
	if err != nil {
		return errors.WithMessage(err, "decode message")
	}

	if err := u.handleMessage(msg); err != nil {
		return errors.WithMessage(err, "handle message")
	}

	return nil
}

func (u *user) OnSessionClosed(session gnet.Session, err error) {
	u.logf("close: %v", err)
	u.stop()
}
