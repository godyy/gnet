package chat

import (
	"encoding/binary"
	"github.com/godyy/gnet"
	"github.com/godyy/gnet/internal/examples/chat/protocol"
	"github.com/pkg/errors"
	pkg_errors "github.com/pkg/errors"
	"math"
	"time"
)

const (
	MsgTypeNotify   = 0 // 通知
	MsgTypeRequest  = 1 // 请求
	MsgTypeResponse = 2 // 响应
)

const (
	ReadTimeout            = 60 * time.Second
	WriteTimeout           = 5 * time.Second
	RequestTimeout         = 10 * time.Second
	MaxLengthOfUserName    = 16
	MaxLengthOfChatContent = 200
)

type Message struct {
	MsgType int8
	MsgSeri uint32
	Protoc  protocol.Protocol
}

func EncodeMessage(msg *Message) (*gnet.Buffer, error) {
	pp := gnet.NewBufferWithCap(1 + 4 + msg.Protoc.Len())
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

func DecodeMessage(pp *gnet.Buffer) (*Message, error) {
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

type PacketReaderWriter struct{}

func (p *PacketReaderWriter) SessionReadPacket(r gnet.ConnReader) (gnet.Packet, error) {
	_ = r.SetReadDeadline(time.Now().Add(ReadTimeout))

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

	return gnet.NewBuffer(body), nil
}

func (_ *PacketReaderWriter) SessionWritePacket(w gnet.ConnWriter, p gnet.Packet, more bool) error {
	n := len(p.Data())
	if n > math.MaxUint16 {
		return errors.New("body length overflow")
	}

	_ = w.SetWriteDeadline(time.Now().Add(WriteTimeout))

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
