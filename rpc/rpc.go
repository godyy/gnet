package rpc

import (
	"encoding/binary"
	"errors"
	"github.com/godyy/gnet"
	"unsafe"
)

// ErrShutdown 表示已关闭
var ErrShutdown = errors.New("rpc: shutdown")

const (
	seqLen         = int(unsafe.Sizeof(uint64(0))) // RPC请求序号字节长度
	methodIdLen    = int(unsafe.Sizeof(uint16(0))) // RPC方法ID字节长度
	seqMethodIdLen = seqLen + methodIdLen          // RPC请求序号和方法ID的组合长度
	requestMinLen  = seqMethodIdLen                // RPC请求数据的最小长度
	responseMinLen = seqMethodIdLen + 1            // RPC响应数据的最小长度
)

// Conn RPC连接
type Conn interface {
	// WritePacket 发送RPC数据包
	WritePacket(p gnet.Packet) error
}

// ErrEncodeBufTooShort 表示编码时的 buffer长度不够.
var ErrEncodeBufTooShort = errors.New("rpc: encode buffer too short")

// requestPacketEncoder 请求数据包编码器.
// 将请求数据包的编码逻辑封装到结构体中，避免使用闭包.
type requestPacketEncoder struct {
	seq      uint64
	methodId uint16
	args     []byte
}

func (enc *requestPacketEncoder) size() int {
	return seqMethodIdLen + len(enc.args)
}

func (enc *requestPacketEncoder) encode(p []byte) error {
	if len(p) < enc.size() {
		return ErrEncodeBufTooShort
	}
	binary.BigEndian.PutUint64(p[:seqLen], enc.seq)
	binary.BigEndian.PutUint16(p[seqLen:seqMethodIdLen], enc.methodId)
	copy(p[seqMethodIdLen:], enc.args)
	return nil
}

// responsePacketEncoder 响应数据包编码器.
// 将响应数据包的编码逻辑封装到结构体中，避免使用闭包.
type responsePacketEncoder struct {
	seq      uint64
	methodId uint16
	err      string
	reply    []byte
}

func (enc *responsePacketEncoder) size() int {
	s := seqMethodIdLen + 1
	if enc.err == "" {
		s += len(enc.reply)
	} else {
		s += len(enc.err)
	}
	return s
}

func (enc *responsePacketEncoder) encode(p []byte) error {
	if len(p) < enc.size() {
		return ErrEncodeBufTooShort
	}
	binary.BigEndian.PutUint64(p[:seqLen], enc.seq)
	binary.BigEndian.PutUint16(p[seqLen:seqMethodIdLen], enc.methodId)
	if enc.err == "" {
		p[seqMethodIdLen] = 0
		copy(p[seqMethodIdLen+1:], enc.reply)
	} else {
		p[seqMethodIdLen] = 1
		copy(p[seqMethodIdLen+1:], enc.err)
	}
	return nil
}
