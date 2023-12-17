package client

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/godyy/gnet"
	"github.com/godyy/gnet/internal/examples/chat"
	"github.com/godyy/gnet/internal/examples/chat/protocol"
	"github.com/pkg/errors"
)

var ErrRequestTimeout = errors.New("request timeout")

type Handler interface {
	OnNotify(protocol.Protocol)
	OnClose(error)
}

type Client struct {
	mtx                 sync.RWMutex
	userName            string
	session             *gnet.TCPSession
	msgSeri             uint32
	requests            map[uint32]*Request
	requestHeap         *requestHeap
	requestTimeoutTimer *time.Timer
	handler             Handler
	heartbeatTimer      *time.Timer
}

func NewClient(handler Handler) *Client {
	return &Client{
		requests:    map[uint32]*Request{},
		requestHeap: newRequestHeap(10),
		handler:     handler,
	}
}

func (c *Client) Start(session *gnet.TCPSession, userName string, opt gnet.TcpSessionCfgReadonly) error {
	if userName == "" {
		return errors.New("user name empty")
	}
	if utf8.RuneCountInString(userName) > chat.MaxLengthOfUserName {
		return errors.New("length of user name exceed limit")
	}

	if err := session.Start(opt, c); err != nil {
		return errors.WithMessage(err, "session start")
	}
	c.session = session

	req, err := c.SendRequest(&protocol.LoginReq{UserName: userName})
	if err != nil {
		return errors.WithMessage(err, "send login request")
	}

	rsp, err := req.WaitResponse()
	if err != nil {
		return errors.WithMessage(err, "wait login response")
	}

	loginRsp, ok := rsp.(*protocol.LoginRsp)
	if !ok {
		return errors.New("response not LoginResponse")
	}
	if loginRsp.Error != "" {
		return errors.New(loginRsp.Error)
	}

	c.userName = userName
	c.startHeartbeat()

	return nil
}

func (c *Client) addRequest(req *Request) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.requestHeap.push(req)
	c.requests[req.msgSeri] = req
	c.setRequestTimeoutTimer()
}

func (c *Client) remRequest(msgSeri uint32) *Request {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	req := c.requests[msgSeri]
	if req == nil {
		return req
	}
	c.requestHeap.remove(req.heapIdx)
	delete(c.requests, msgSeri)
	c.setRequestTimeoutTimer()
	return req
}

func (c *Client) setRequestTimeoutTimer() {
	if c.requestTimeoutTimer != nil {
		c.requestTimeoutTimer.Stop()
	}

	if c.requestHeap.len() == 0 {
		return
	}

	req := c.requestHeap.top()
	c.requestTimeoutTimer = time.AfterFunc(req.timeout.Sub(time.Now()), c.onRequestTimeout)
}

func (c *Client) onRequestTimeout() {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.requestHeap.len() == 0 {
		c.setRequestTimeoutTimer()
		return
	}

	for c.requestHeap.len() > 0 {
		req := c.requestHeap.top()
		if req.timeout.After(time.Now()) {
			break
		}

		c.requestHeap.pop()
		delete(c.requests, req.msgSeri)
		req.error(ErrRequestTimeout)
	}

	c.setRequestTimeoutTimer()
}

func (c *Client) startHeartbeat() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.heartbeatTimer != nil {
		c.heartbeatTimer.Stop()
		c.heartbeatTimer = nil
	}

	c.heartbeatTimer = time.AfterFunc(chat.ReceiveTimeout/2, c.onHeartbeat)
}

func (c *Client) onHeartbeat() {
	_ = c.sendMessage(&chat.Message{
		MsgType: chat.MsgTypeNotify,
		MsgSeri: 0,
		Protoc:  &protocol.HeartbeatReq{},
	})
}

func (c *Client) SendRequest(protoc protocol.Protocol, timeout ...time.Duration) (*Request, error) {
	msgSeri := atomic.AddUint32(&c.msgSeri, 1)
	msg := &chat.Message{
		MsgType: chat.MsgTypeRequest,
		MsgSeri: msgSeri,
		Protoc:  protoc,
	}
	expireTime := time.Now()
	if len(timeout) > 0 {
		expireTime = expireTime.Add(timeout[0])
	} else {
		expireTime = expireTime.Add(chat.RequestTimeout)
	}
	req := newRequest(msgSeri, expireTime)
	c.addRequest(req)

	if err := c.sendMessage(msg); err != nil {
		c.remRequest(msgSeri)
		req.error(err)
		return nil, errors.WithMessage(err, "send message")
	}

	return req, nil
}

func (c *Client) sendMessage(msg *chat.Message) error {
	packet, err := chat.EncodeMessage(msg)
	if err != nil {
		return errors.WithMessage(err, "encode message")
	}
	err = c.session.SendPacket(packet)
	c.startHeartbeat()
	return err
}

func (c *Client) OnSessionPacket(session gnet.Session, packet *gnet.Packet) error {
	defer gnet.PutPacket(packet)

	msg, err := chat.DecodeMessage(packet)
	if err != nil {
		// todo
		return errors.WithMessage(err, "decode message")
	}

	switch msg.MsgType {
	case chat.MsgTypeNotify:
		c.handler.OnNotify(msg.Protoc)
	case chat.MsgTypeResponse:
		c.handleResponse(msg)
	default:
		return fmt.Errorf("invalid message type %d", msg.MsgType)
	}

	return nil
}

func (c *Client) handleResponse(msg *chat.Message) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if req, ok := c.requests[msg.MsgSeri]; ok {
		req.response(msg.Protoc)
		c.requestHeap.remove(req.heapIdx)
		delete(c.requests, msg.MsgSeri)
	}
}

func (c *Client) OnSessionClosed(session gnet.Session, err error) {
	c.handler.OnClose(err)
}
