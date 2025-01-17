package gnet

import (
	"errors"
	"net"
	"sync/atomic"
)

// ErrSessionStarted 表示 Session 已启动
var ErrSessionStarted = errors.New("gnet: session started")

// ErrSessionClosed 表示 Session 已关闭
var ErrSessionClosed = errors.New("gnet: session closed")

// SessionHandler Session 处理器
type SessionHandler interface {
	// SessionPendingPacket 获取待发送的数据包
	// p 为获取到的数据包，more 表示是否还有更多数据包需要发送.
	SessionPendingPacket() (p Packet, more bool, err error)

	// SessionOnPacket 数据包回调
	SessionOnPacket(s *Session, p Packet) error

	// SessionOnClosed Session 关闭回调
	SessionOnClosed(s *Session, err error)
}

// SessionPacketReader Session 数据包读取功能
type SessionPacketReader interface {
	// SessionReadPacket 从 r 中读取数据包
	SessionReadPacket(r ConnReader) (Packet, error)
}

// SessionPacketWriter Session 数据包写入功能
type SessionPacketWriter interface {
	// SessionWritePacket 将 p 写入 w
	// more 表示是否还有更多数据包需要发送.
	SessionWritePacket(w ConnWriter, p Packet, more bool) error
}

const (
	sessionIdle    = 0
	sessionStarted = 1
	sessionClosed  = 2
)

// Session 网络会话
// 将数据包的的读写和处理流程抽象出来，使用户可以根据不同的需求实现独特的
// 数据包处理流程，同时提供通用的数据包读写协程支持.
type Session struct {
	state        atomic.Int32        // 状态
	conn         net.Conn            // 网络连接对象
	packetReader SessionPacketReader // 数据包读取工具
	packetWriter SessionPacketWriter // 数据包写入工具
	handler      SessionHandler      //
}

// NewSession 构造 Session
func NewSession(conn net.Conn, packetReader SessionPacketReader, packetWriter SessionPacketWriter, handler SessionHandler) *Session {
	if conn == nil {
		panic("gnet.NewSession: conn is nil")
	}
	if handler == nil {
		panic("gnet.NewSession: handler is nil")
	}
	if packetReader == nil {
		panic("gnet.NewSession: packetReader is nil")
	}
	if packetWriter == nil {
		panic("gnet.NewSession: packetWriter is nil")
	}

	return &Session{
		conn:         conn,
		packetReader: packetReader,
		packetWriter: packetWriter,
		handler:      handler,
	}
}

// Start 启动异步读写
func (s *Session) Start() error {
	if s.state.CompareAndSwap(sessionIdle, sessionStarted) {
		go s.readPacket()
		go s.writePacket()
		return nil
	} else {
		return ErrSessionStarted
	}
}

// LocalAddr 返回 Session 相关网络连接本地地址
func (s *Session) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

// RemoteAddr 返回 Session 相关网络连接远端地址
func (s *Session) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

// Handler 返回 Session 的处理器对象
func (s *Session) Handler() SessionHandler {
	return s.handler
}

// Close 关闭 Session
// 关闭关联的网络连接, 终止后台读写协程.
func (s *Session) Close() error {
	return s.close(ErrSessionClosed)
}

// close 关闭 Session 的实现
// err 表示导致会话关闭的原因.
func (s *Session) close(err error) error {
	if s.state.CompareAndSwap(sessionStarted, sessionClosed) {
		_ = s.conn.Close()
		s.handler.SessionOnClosed(s, err)
		return nil
	} else {
		return ErrSessionClosed
	}
}

// closed 返回会话是否已关闭
func (s *Session) closed() bool {
	return s.state.Load() == sessionClosed
}

// readPacket 数据包读取协程
func (s *Session) readPacket() {
	var (
		p   Packet
		err error
	)

	for !s.closed() {
		if p, err = s.packetReader.SessionReadPacket(s.conn); err != nil {
			break
		}

		if err = s.handler.SessionOnPacket(s, p); err != nil {
			break
		}
	}

	if err != nil {
		_ = s.close(err)
	}
}

// writePacket 数据包写入协程
func (s *Session) writePacket() {
	var (
		p    Packet
		more bool
		err  error
	)

	for !s.closed() {
		if p, more, err = s.handler.SessionPendingPacket(); err != nil {
			break
		}

		if err = s.packetWriter.SessionWritePacket(s.conn, p, more); err != nil {
			break
		}
	}

	if err != nil {
		_ = s.close(err)
	}
}
