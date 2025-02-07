package rpc

import (
	"encoding/binary"
	"errors"
	"github.com/godyy/gnet"
	pkg_errors "github.com/pkg/errors"
)

// ServerHandler 封装RPC服务端需要实现的功能
type ServerHandler interface {
	// RPCDecodeMethodArgs 解码方法arguments.
	RPCDecodeMethodArgs(methodId uint16, argsBytes []byte) (any, error)

	ResponsePacketEncoder

	// RPCHandleRequest 处理RPC请求
	RPCHandleRequest(req *Request) error
}

// Server RPC服务端
type Server struct {
	handler ServerHandler // 服务端功能实现
}

// NewServer 创建RPC服务端
func NewServer(h ServerHandler) *Server {
	if h == nil {
		panic("rpc.NewServer: server handler nil")
	}

	srv := &Server{
		handler: h,
	}

	return srv
}

// ErrRequestTooShort 表示RPC请求数据长度过短
var ErrRequestTooShort = errors.New("rpc: request data too short")

// HandleRequest 处理来自 conn 所表示RPC连接的，data 所表示的调用请求.
func (s *Server) HandleRequest(conn Conn, data []byte) (err error) {
	var req Request

	if len(data) <= requestMinLen {
		return ErrRequestTooShort
	}

	// 解码请求序号
	req.Seq = binary.BigEndian.Uint64(data[:seqLen])

	// 解码方法ID
	req.MethodId = binary.BigEndian.Uint16(data[seqLen:seqMethodIdLen])

	// 解码 arguments
	req.Args, err = s.handler.RPCDecodeMethodArgs(req.MethodId, data[seqMethodIdLen:])
	if err != nil {
		return pkg_errors.WithMessagef(err, "rpc: decode method:%d args", req.MethodId)
	}

	req.conn = conn
	req.responseEncoder = s.handler
	return s.handler.RPCHandleRequest(&req)
}

// ResponsePacketEncoder 封装编码响应数据包需要实现的逻辑.
type ResponsePacketEncoder interface {
	// RPCEncodeMethodReply 编码方法reply.
	RPCEncodeMethodReply(methodId uint16, reply any) ([]byte, error)

	// RPCEncodeResponsePacket 编码 response 数据包.
	// 根据 size 指定的大小创建请求数据包，并将以数据包缓冲区为参数调用 encoder，
	// 在 encoder 内部进行数据填充.
	RPCEncodeResponsePacket(size int, encoder func(p []byte) error) (gnet.Packet, error)
}

// Request RPC请求
type Request struct {
	Seq      uint64 // seq.
	MethodId uint16 // method id.
	Args     any    // arguments.

	conn            Conn                  // 接收 request 的连接，用来发送 response.
	responseEncoder ResponsePacketEncoder // 响应数据包编码实现
	responded       bool                  // 是否已响应.
}

// ErrAlreadyResponded 表示请求已回复.
var ErrAlreadyResponded = errors.New("rpc: request already responded")

// checkResponded 检查请求是否回复
func (req *Request) checkResponded() error {
	if req.responded {
		return ErrAlreadyResponded
	}
	return nil
}

// respond 根据 encoder 编码响应并发送.
func (req *Request) respond(encoder *responsePacketEncoder) error {
	p, err := req.responseEncoder.RPCEncodeResponsePacket(encoder.size(), encoder.encode)
	if err != nil {
		return pkg_errors.WithMessage(err, "rpc: encode response packet")
	}

	if err := req.conn.WritePacket(p); err != nil {
		return pkg_errors.WithMessage(err, "rpc: send response")
	}

	req.responded = true
	return nil
}

// Reply 调用成功，回复 reply.
func (req *Request) Reply(reply any) error {
	if err := req.checkResponded(); err != nil {
		return err
	}

	replyBytes, err := req.responseEncoder.RPCEncodeMethodReply(req.MethodId, reply)
	if err != nil {
		return pkg_errors.WithMessagef(err, "rpc: encode method:%d reply", req.MethodId)
	}

	encoder := responsePacketEncoder{
		seq:      req.Seq,
		methodId: req.MethodId,
		reply:    replyBytes,
	}

	return req.respond(&encoder)
}

// Error 调用失败，回复 err 表示的错误.
func (req *Request) Error(err error) error {
	if err == nil {
		panic("rpc: reply nil error")
	}

	if err := req.checkResponded(); err != nil {
		return err
	}

	encoder := responsePacketEncoder{
		seq:      req.Seq,
		methodId: req.MethodId,
		err:      err.Error(),
	}

	return req.respond(&encoder)
}
