package gnet

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// ErrSessionNotStarted 会话未启动
var ErrSessionNotStarted = errors.New("gnet: session not started")

// ErrSessionStarted 会话已启动
var ErrSessionStarted = errors.New("gnet: session started")

// ErrSessionClosing 会话正在关闭
var ErrSessionClosing = errors.New("gnet: session closing")

// ErrSessionClosed 会话已关闭
var ErrSessionClosed = errors.New("gnet: session closed")

// ErrSendQueueFull 发送队列已满
var ErrSendQueueFull = errors.New("gnet: send queue is full")

// ErrPacketSizeOutOfRange 消息包大小超过限制
var ErrPacketSizeOutOfRange = errors.New("gnet: packet size is out of range")

const (
	sessionInit    = iota // 会话初始化
	sessionStarted        // 会话已启动
	sessionClosing        // 会话正在关闭
	sessionClosed         // 会话已关闭
)

func getSessionStateErr(state int32) error {
	switch state {
	case sessionInit:
		return ErrSessionNotStarted
	case sessionStarted:
		return ErrSessionStarted
	case sessionClosing:
		return ErrSessionClosing
	case sessionClosed:
		return ErrSessionClosed
	default:
		panic(fmt.Sprintf("gnet.TCPSession: illegal session state %d", state))
	}
}

// Session 会话interface, 一个会话需要实现的最小方法集。
type Session interface {
	// LocalAddr 返回本地地址
	LocalAddr() net.Addr

	// RemoteAddr 返回对端地址
	RemoteAddr() net.Addr

	// UserData 获取UserData
	// 会话关闭后清理
	UserData() any

	// SetUserData 设置UserData，同时返回之前的数据
	// 会话关闭后设置无效，且直接返回当前数据
	SetUserData(u any) any

	// Handler 获取会话处理器
	// 会话关闭后清理
	Handler() SessionHandler

	// Start 启动会话
	Start(SessionHandler) error

	// Close 关闭
	Close(error) error

	// SendPacket 发送数据包
	SendPacket(CustomPacket) error

	// SendData 发送数据
	SendData([]byte) error
}

// SessionHandler 会话事件处理器
type SessionHandler interface {
	// GetPacket 通过指定大小获取自定义数据包
	// CustomPacket.Data() 获取的字节切片的长度为size.
	// 若返回nil，默认会创建大小为size的RawPacket.
	GetPacket(size int) CustomPacket

	// PutPacket 回收自定义数据包
	PutPacket(CustomPacket)

	// OnSessionPacket 接收数据包事件
	OnSessionPacket(Session, CustomPacket) error

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

// 内部duration单位 ms
const durationUnit = time.Millisecond

func time2Duration(t int) time.Duration {
	return time.Duration(t) * durationUnit
}
