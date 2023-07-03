package gnet

import (
	"errors"
	"net"
	"time"
)

// ErrSessionNotStarted 会话未启动
var ErrSessionNotStarted = errors.New("gnet: session not started")

// ErrSessionStarted 会话已启动
var ErrSessionStarted = errors.New("gnet: session started")

// ErrSessionClosed 会话已关闭
var ErrSessionClosed = errors.New("gnet: session closed")

// ErrSendQueueFull 发送队列已满
var ErrSendQueueFull = errors.New("gnet: send queue is full")

// ErrPacketSizeOutOfRange 消息包大小超过限制
var ErrPacketSizeOutOfRange = errors.New("gnet: packet size is out of range")

const (
	SessionIdle    = iota // 会话未启动
	SessionStarted        // 会话已启动
	SessionClosed         // 会话已关闭
)

// Session 会话interface, 一个会话需要实现的最小方法集。
type Session interface {
	// State 获取会话当前所处状态
	State() int32

	// LocalAddr 返回本地地址
	LocalAddr() net.Addr

	// RemoteAddr 返回对端地址
	RemoteAddr() net.Addr

	// Handler 获取会话处理器
	Handler() SessionHandler

	// SendPacket 发送数据包
	SendPacket(*Packet, ...time.Duration) error

	// Close 关闭
	Close(error) error
}

// SessionHandler 会话事件处理器
type SessionHandler interface {
	// OnSessionPacket 接收数据包事件
	OnSessionPacket(Session, *Packet) error

	// OnSessionClosed 会话关闭事件
	OnSessionClosed(Session, error)
}

// Listener 网络监听器，监听外来网络连接
type Listener interface {
	// Start 开始监听
	Start(func(conn net.Conn)) error

	// Close 关闭监听
	Close() error
}
