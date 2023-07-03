package protocol

import (
	"fmt"
	"github.com/godyy/gnet"
	"github.com/pkg/errors"
	"reflect"
)

type Protocol interface {
	Len() int
	Encode(*gnet.Packet) error
	Decode(*gnet.Packet) error
}

var protocolTypes = map[string]reflect.Type{}

func registerProtocolType(p Protocol) {
	pt := reflect.TypeOf(p).Elem()
	protocolTypes[pt.Name()] = pt
}

func createProtocol(ts string) Protocol {
	if t, ok := protocolTypes[ts]; ok {
		return reflect.New(t).Interface().(Protocol)
	} else {
		return nil
	}
}

type LoginReq struct {
	UserName string
}

func (l *LoginReq) Len() int {
	return 2 + len(l.UserName)
}

func (l *LoginReq) Encode(p *gnet.Packet) (err error) {
	err = p.WriteString(l.UserName)
	if err != nil {
		return errors.WithMessage(err, "encode UserName")
	}
	return
}

func (l *LoginReq) Decode(p *gnet.Packet) (err error) {
	l.UserName, err = p.ReadString()
	if err != nil {
		return errors.WithMessage(err, "decode UserName")
	}
	return
}

type LoginRsp struct {
	Error string
}

func (l *LoginRsp) Len() int {
	return 2 + len(l.Error)
}

func (l *LoginRsp) Encode(p *gnet.Packet) (err error) {
	err = p.WriteString(l.Error)
	if err != nil {
		return errors.WithMessage(err, "encode Error")
	}
	return
}

func (l *LoginRsp) Decode(p *gnet.Packet) (err error) {
	l.Error, err = p.ReadString()
	if err != nil {
		return errors.WithMessage(err, "decode Error")
	}
	return
}

type SendMessageReq struct {
	Message string
}

func (s *SendMessageReq) Len() int {
	return 2 + len(s.Message)
}

func (s *SendMessageReq) Encode(p *gnet.Packet) (err error) {
	err = p.WriteString(s.Message)
	if err != nil {
		return errors.WithMessage(err, "encode Message")
	}
	return
}

func (s *SendMessageReq) Decode(p *gnet.Packet) (err error) {
	s.Message, err = p.ReadString()
	if err != nil {
		return errors.WithMessage(err, "decode Message")
	}
	return
}

type SendMessageRsp struct {
	Error string
}

func (s *SendMessageRsp) Len() int {
	return 2 + len(s.Error)
}

func (s *SendMessageRsp) Encode(p *gnet.Packet) (err error) {
	err = p.WriteString(s.Error)
	if err != nil {
		return errors.WithMessage(err, "encode Error")
	}
	return
}

func (s *SendMessageRsp) Decode(p *gnet.Packet) (err error) {
	s.Error, err = p.ReadString()
	if err != nil {
		return errors.WithMessage(err, "decode Error")
	}
	return
}

const (
	MsgTypeUserEnter   = 1
	MsgTypeUserExit    = 2
	MsgTypeUserMessage = 3
)

type MessageNotify struct {
	Type     int8
	UserName string
	Message  string
	TimeMS   int64
}

func (r *MessageNotify) Len() int {
	return 1 + 2 + len(r.UserName) + 2 + len(r.Message) + 8
}

func (r *MessageNotify) Encode(p *gnet.Packet) (err error) {
	if err = p.WriteInt8(r.Type); err != nil {
		return errors.WithMessage(err, "encode Type")
	}

	if err = p.WriteString(r.UserName); err != nil {
		return errors.WithMessage(err, "encode UserName")
	}

	if err = p.WriteString(r.Message); err != nil {
		return errors.WithMessage(err, "encode Message")
	}

	if err = p.WriteInt64(r.TimeMS); err != nil {
		return errors.WithMessage(err, "encode TimeMS")
	}

	return
}

func (r *MessageNotify) Decode(p *gnet.Packet) (err error) {
	if r.Type, err = p.ReadInt8(); err != nil {
		return errors.WithMessage(err, "decode Type")
	}

	if r.UserName, err = p.ReadString(); err != nil {
		return errors.WithMessage(err, "decode UserName")
	}

	if r.Message, err = p.ReadString(); err != nil {
		return errors.WithMessage(err, "decode Message")
	}

	if r.TimeMS, err = p.ReadInt64(); err != nil {
		return errors.WithMessage(err, "decode TimeMS")
	}

	return
}

func Encode(p Protocol, pp *gnet.Packet) error {
	protocolType := reflect.TypeOf(p).Elem().Name()
	if _, ok := protocolTypes[protocolType]; !ok {
		return fmt.Errorf("Protocol Type %s wrong", protocolType)
	}

	if err := pp.WriteString(protocolType); err != nil {
		return errors.WithMessage(err, "encode Protocol Type")
	}

	if err := p.Encode(pp); err != nil {
		return errors.WithMessage(err, "encode Protocol Data")
	}

	return nil
}

func Decode(pp *gnet.Packet) (Protocol, error) {
	protocolType, err := pp.ReadString()
	if err != nil {
		return nil, errors.WithMessage(err, "decode Protocol Type")
	}

	p := createProtocol(protocolType)
	if p == nil {
		return nil, fmt.Errorf("Protocol Type %s wrong", protocolType)
	}

	if err := p.Decode(pp); err != nil {
		return nil, errors.WithMessage(err, "decode Protocol Data")
	}

	return p, nil
}

type HeartbeatReq struct{}

func (h *HeartbeatReq) Len() int {
	return 1
}

func (h *HeartbeatReq) Encode(packet *gnet.Packet) error {
	if err := packet.WriteByte(0); err != nil {
		return err
	}
	return nil
}

func (h *HeartbeatReq) Decode(packet *gnet.Packet) error {
	_, err := packet.ReadByte()
	return err
}

type HeartbeatRsp struct{}

func (h *HeartbeatRsp) Len() int {
	return 1
}

func (h *HeartbeatRsp) Encode(packet *gnet.Packet) error {
	if err := packet.WriteByte(0); err != nil {
		return err
	}
	return nil
}

func (h *HeartbeatRsp) Decode(packet *gnet.Packet) error {
	_, err := packet.ReadByte()
	return err
}

func init() {
	registerProtocolType(&LoginReq{})
	registerProtocolType(&LoginRsp{})
	registerProtocolType(&SendMessageReq{})
	registerProtocolType(&SendMessageRsp{})
	registerProtocolType(&MessageNotify{})
	registerProtocolType(&HeartbeatReq{})
	registerProtocolType(&HeartbeatRsp{})
}
