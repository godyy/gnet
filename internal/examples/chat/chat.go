package chat

import (
	"time"

	"github.com/godyy/gnet"
	"github.com/godyy/gnet/internal/examples/chat/protocol"
	"github.com/pkg/errors"
)

const (
	MsgTypeNotify   = 0 // 通知
	MsgTypeRequest  = 1 // 请求
	MsgTypeResponse = 2 // 响应
)

const (
	ReceiveTimeout         = 60 * 1000
	SendTimeout            = 5 * 1000
	RequestTimeout         = 10 * time.Second
	MaxLengthOfUserName    = 16
	MaxLengthOfChatContent = 200
)

type Message struct {
	MsgType int8
	MsgSeri uint32
	Protoc  protocol.Protocol
}

func EncodeMessage(msg *Message) (*gnet.Packet, error) {
	pp := gnet.GetPacket(1 + 4 + msg.Protoc.Len())
	if err := pp.WriteInt8(msg.MsgType); err != nil {
		return nil, errors.WithMessage(err, "write msg-type")
	}
	if err := pp.WriteUint32(msg.MsgSeri); err != nil {
		return nil, errors.WithMessage(err, "write msg-seri")
	}
	if err := protocol.Encode(msg.Protoc, pp); err != nil {
		return nil, errors.WithMessage(err, "encode protocol")
	}
	return pp, nil
}

func DecodeMessage(pp *gnet.Packet) (*Message, error) {
	msgType, err := pp.ReadInt8()
	if err != nil {
		return nil, errors.WithMessage(err, "read msg-type")
	}
	msgSeri, err := pp.ReadUint32()
	if err != nil {
		return nil, errors.WithMessage(err, "read msg-seri")
	}
	p, err := protocol.Decode(pp)
	if err != nil {
		return nil, errors.WithMessage(err, "decode protocol")
	}
	return &Message{
		MsgType: msgType,
		MsgSeri: msgSeri,
		Protoc:  p,
	}, nil
}
