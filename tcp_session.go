package gnet

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/godyy/gutils/buffer"

	"github.com/godyy/gutils/buffer/bytes"
	pkg_errors "github.com/pkg/errors"
)

const (
	// tcp消息包包体大小字节长度
	tcpPacketSizeLen = 4

	// tcp会话默认接收缓冲区大小
	tcpDefaultReceiveBufferSize = 4096

	// tcp会话默认发送缓冲区大小
	tcpDefaultSendBufferSize = 4096
)

// TcpSessionCfg Tcp会话配置
type TcpSessionCfg struct {
	// ReceiveTimeout 接收超时 ms
	// default zero, mean no limit.
	ReceiveTimeout int

	// SendTimeout 发送超时 ms
	// default zero, mean no limit.
	SendTimeout int

	// ReceiveBufferSize 接收缓冲区大小
	// default 4096.
	ReceiveBufferSize int

	// SendBufferSize 发送缓冲区大小
	// default 4096.
	SendBufferSize int

	// MaxPacketSize 最大消息包大小
	// 接收到超过此上限的消息包，会默认连接非法.
	// 理论上限为 MaxUint32，default 4092 = 4096 - 4.
	MaxPacketSize int
}

func NewTcpSessionCfg() *TcpSessionCfg {
	return &TcpSessionCfg{
		ReceiveTimeout:    0,
		SendTimeout:       0,
		ReceiveBufferSize: tcpDefaultReceiveBufferSize,
		SendBufferSize:    tcpDefaultSendBufferSize,
		MaxPacketSize:     tcpDefaultReceiveBufferSize - tcpPacketSizeLen,
	}
}

func (c *TcpSessionCfg) ReceiveTimeoutDuration() time.Duration {
	return time2Duration(c.ReceiveTimeout)
}

func (c *TcpSessionCfg) SendTimeoutDuration() time.Duration {
	return time2Duration(c.SendTimeout)
}

// TCPSession TCP网络会话
type TCPSession struct {
	locker            *sync.RWMutex      // locker
	state             int32              // 会话状态
	conn              *net.TCPConn       // tcp conn
	cfg               *TcpSessionCfg     // 会话配置，用于读取会话设置
	pUserData         *any               // 存储与该会话唯一关联的数据
	pHandler          *SessionHandler    // 会话处理器
	pendingPacketCond *sync.Cond         // 发送条件信号
	pendingPacketQue  *PacketQueue       // 待发送数据包队列
	sendingPacketQue  *PacketQueue       // 正发送数据包队列
	sendBuf           *bytes.FixedBuffer // 发送缓冲区
	receiveBuf        *bytes.FixedBuffer // 接收缓冲区
	closeFlag         int32              // 关闭标记
	closeErr          error              // 造成会话关闭的error
}

// NewTCPSession 创建TCPSession
// conn 为底层TCP连接，cfg 提供TCPSession使用的配置参数，h 为会话事件处理处理器
func NewTCPSession(conn *net.TCPConn, cfg *TcpSessionCfg) *TCPSession {
	if conn == nil {
		panic("gnet.NewTCPSession: conn nil")
	}

	if cfg == nil {
		panic("gnet.NewTCPSession: cfg nil")
	}

	s := &TCPSession{
		locker: &sync.RWMutex{},
		state:  sessionInit,
		conn:   conn,
		cfg:    cfg,
	}
	return s
}

// Start 启动会话
func (s *TCPSession) Start(handler SessionHandler) error {
	if handler == nil {
		panic("gnet.NewTCPSession: handler nil")
	}

	s.locker.Lock()
	defer s.locker.Unlock()

	if s.state != sessionInit {
		return getSessionStateErr(s.state)
	}

	s.setHandler(handler)
	s.pendingPacketCond = sync.NewCond(s.locker)
	s.pendingPacketQue = NewPacketQueue()
	s.sendingPacketQue = NewPacketQueue()
	s.state = sessionStarted
	go s.receiveLoop()
	go s.sendLoop()
	return nil
}

// LocalAddr 本会本地地址
func (s *TCPSession) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

// RemoteAddr 返回对端地址
func (s *TCPSession) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

// UserData 获取UserData
func (s *TCPSession) UserData() any {
	ptr := (*any)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.pUserData))))
	if ptr == nil {
		return ptr
	}
	return *ptr
}

// SetUserData 更新UserData
func (s *TCPSession) SetUserData(u any) any {
	if s.state >= sessionClosing {
		return s.UserData()
	}

	ptr := new(any)
	*ptr = u
	oldPtr := (*any)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.pUserData)), unsafe.Pointer(ptr)))

	if oldPtr == nil {
		return nil
	}

	return *oldPtr
}

// Handler 获取handler
func (s *TCPSession) Handler() SessionHandler {
	ptr := (*SessionHandler)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.pHandler))))
	if ptr == nil {
		return nil
	}
	return *ptr
}

// setHandler 设置Handler
func (s *TCPSession) setHandler(h SessionHandler) {
	ptr := new(SessionHandler)
	*ptr = h
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s.pHandler)), unsafe.Pointer(ptr))
}

// SendPacket 发送数据包
// p 所提供的消息包不会立即发送，而是被放入发送队列中，等待发送goroutine提取并发送。
// 如果 p 的大小超过 cfg.GetMaxPacketSize() 或为0，返回 ErrPacketSizeOutOfRange。
// 如果会话已经关闭，返回 ErrSessionClosed。
func (s *TCPSession) SendPacket(p CustomPacket) error {
	if p == nil {
		panic("gnet.TCPSession.SendPacket: p nil")
	}

	s.locker.Lock()
	defer s.locker.Unlock()

	if s.state != sessionStarted {
		return getSessionStateErr(s.state)
	}

	if size := len(p.Data()); size == 0 || size > s.cfg.MaxPacketSize {
		return ErrPacketSizeOutOfRange
	}

	s.pendingPacketQue.Push(p)
	s.pendingPacketCond.Signal()
	return nil
}

// SendData 发送数据
func (s *TCPSession) SendData(data []byte) error {
	if len(data) <= 0 {
		return nil
	}

	return s.SendPacket(RawPacket(data))
}

// Close 关闭会话
// reason 用于提供关闭会话的原因，该 reason 在会话关闭后，可以通过 CloseErr() 获取，
// 但前提是调用 Close(reason) 之前，会话并未关闭.
func (s *TCPSession) Close(reason error) error {
	s.locker.Lock()
	defer s.locker.Unlock()

	if s.state != sessionStarted {
		return getSessionStateErr(s.state)
	}

	s.closeErr = reason
	_ = s.conn.Close()
	s.pendingPacketCond.Signal()
	s.state = sessionClosing
	return nil
}

// loopClose 内部循环关闭
func (s *TCPSession) loopClose(err error) {
	s.Close(err)
	if atomic.AddInt32(&s.closeFlag, 1) >= 2 {
		s.state = sessionClosed
		(*s.pHandler).OnSessionClosed(s, s.closeErr)
		s.clear()
	}
}

func (s *TCPSession) clear() {
	s.pendingPacketQue.Clear()
	s.sendingPacketQue.Clear()
	s.SetUserData(nil)
	s.setHandler(nil)
	s.cfg = nil
}

// sendLoop 发送循环
// sendLoop 在发送数据出错，或会话关闭后，会自动退出。
func (s *TCPSession) sendLoop() {
	var err error

	s.sendBuf = bytes.NewFixedBuffer(s.cfg.SendBufferSize)

	for {
		if s.state != sessionStarted {
			err = getSessionStateErr(s.state)
			break
		}

		s.swapPendingPacketQue()
		if err = s.sendPacketQueue(); err != nil {
			break
		}
	}

	// 尽量保证缓存数据能发送出去
	_ = s.sendBuffered(true)

	s.sendBuf = nil
	s.loopClose(err)
}

// swapPendingPacketQue 置换待发送消息队列
func (s *TCPSession) swapPendingPacketQue() {
	s.locker.Lock()
	defer s.locker.Unlock()
	for {
		if s.pendingPacketQue.len > 0 {
			s.pendingPacketQue, s.sendingPacketQue = s.sendingPacketQue, s.pendingPacketQue
			return
		}

		if s.state != sessionStarted {
			return
		}

		s.pendingPacketCond.Wait()
	}
}

// sendPacketQueue 发送消息包队列
func (s *TCPSession) sendPacketQueue() (err error) {
	if s.sendingPacketQue.len <= 0 {
		return nil
	}

	for p := s.sendingPacketQue.Pop(); p != nil; p = s.sendingPacketQue.Pop() {
		if err = s.writePacket(p); err != nil {
			return
		}

		s.putPacket(p)
	}

	if err = s.sendBuffered(true); err != nil {
		err = pkg_errors.WithMessage(err, "gnet.TCPSession: send data buffered")
	}

	return
}

// writePacket 将数据包写入发送缓冲区
func (s *TCPSession) writePacket(p CustomPacket) error {
	var err error

	data := p.Data()

	// 读取包大小
	size := len(data)

	// 写包大小
	for s.sendBuf.Writable() < tcpPacketSizeLen {
		if err = s.sendBuffered(false); err != nil {
			return pkg_errors.WithMessage(err, "gnet.TCPSession: write packet size: send data buffered")
		}
	}
	err = s.sendBuf.WriteUint32(uint32(size))
	if err != nil {
		// 将包大小写入缓存失败
		return pkg_errors.WithMessage(err, "gnet.TCPSession: write packet size to buffer")
	}

	// 写包体
	var n int
	var w int
	for w < size {
		n, err = s.sendBuf.Write(data[w:])
		if err != nil && err != buffer.ErrBufferFull {
			// 将包数据写入缓存失败
			return pkg_errors.WithMessage(err, "gnet.TCPSession: write packet data to buffer")
		}

		w += n
		if s.sendBuf.Writable() < tcpPacketSizeLen {
			if err = s.sendBuffered(false); err != nil {
				return pkg_errors.WithMessage(err, "gnet.TCPSession: send data buffered")
			}
		}
	}

	return err
}

// sendBuffered 发送缓存在发送缓冲区中的数据
// all 用于控制是否确保所有缓冲区中的数据都发送成功。
func (s *TCPSession) sendBuffered(all bool) (err error) {
	if s.sendBuf.Readable() == 0 {
		return nil
	}

	for {
		// set write timeout
		if sendTimeout := s.cfg.SendTimeoutDuration(); sendTimeout > 0 {
			s.conn.SetWriteDeadline(time.Now().Add(sendTimeout))
		} else {
			s.conn.SetWriteDeadline(time.Time{})
		}

		if _, err = s.sendBuf.WriteTo(s.conn); err != nil {
			// 发送数据出错
			break
		}

		if all {
			if s.sendBuf.Readable() == 0 {
				break
			}
		} else {
			if s.sendBuf.Writable() >= tcpPacketSizeLen {
				break
			}
		}
	}

	return
}

// receiveLoop 接收循环
// 循环的接收数据，生成消息包并推入接收队列。数据会事先读入接收缓冲区中，待缓冲区中数据足够，
// 再生成消息包。接收缓冲区的大小由 cfg.GetReceiveBufferSize 提供。
// receiveLoop 在接收出错或会话关闭后会自动退出。
func (s *TCPSession) receiveLoop() {
	var err error

	// 申请接收缓冲区
	s.receiveBuf = bytes.NewFixedBuffer(s.cfg.ReceiveBufferSize)

	for {
		if s.state != sessionStarted {
			err = getSessionStateErr(s.state)
			break
		}

		if err = s.receivePacket(); err != nil {
			break
		}
	}

	s.receiveBuf = nil
	s.loopClose(err)
}

// receivePacket 接收单个消息包
// 事先将数据读入接收缓冲区中，再通过接收缓冲区中的数据生成消息包，并放入接收队列.
// closed 用于返回会话是否关闭，err 用于返回接收过程中产生的错误。
func (s *TCPSession) receivePacket() (err error) {
	var size int

	for s.receiveBuf.Readable() < tcpPacketSizeLen {
		if err = s.receive2Buffer(); err != nil {
			err = pkg_errors.WithMessage(err, "gnet.TCPSession: receive to buffer")
			return
		}
	}

	if u32, e := s.receiveBuf.ReadUint32(); e != nil {
		// 读取包大小失败
		err = pkg_errors.WithMessage(e, "gnet.TCPSession: read packet size from buffer")
		return
	} else {
		size = int(u32)
		if size == 0 || size > s.cfg.MaxPacketSize {
			// 包大小超出范围
			err = fmt.Errorf("gnet.TCPSession: receive packet size %d out of range", size)
			return
		}
	}

	packet := s.getPacket(size)
	data := packet.Data()
	r := 0
	n := 0
	for {
		n, err = s.receiveBuf.Read(data[r:])
		if err != nil && err != io.EOF {
			err = pkg_errors.WithMessage(err, "gnet.TCPSession: read data from buffer")
			return
		}

		if r += n; r >= size {
			break
		}

		if err = s.receive2Buffer(); err != nil {
			err = pkg_errors.WithMessage(err, "gnet.TCPSession: receive data to buffer")
			return
		}
	}

	err = (*s.pHandler).OnSessionPacket(s, packet)
	return
}

// receive2Buffer 将数据读入接收缓冲区
// err 用于返回读取过程中出现的错误
func (s *TCPSession) receive2Buffer() (err error) {
	// set read timeout
	if receiveTimeout := s.cfg.ReceiveTimeoutDuration(); receiveTimeout > 0 {
		s.conn.SetReadDeadline(time.Now().Add(receiveTimeout))
	} else {
		s.conn.SetReadDeadline(time.Time{})
	}

	// 读取数据并放入缓存
	_, err = s.receiveBuf.ReadFrom(s.conn)
	return
}

func (s *TCPSession) getPacket(size int) CustomPacket {
	p := (*s.pHandler).GetPacket(size)
	if p == nil {
		p = RawPacket(make([]byte, size))
	}
	return p
}

func (s *TCPSession) putPacket(p CustomPacket) {
	(*s.pHandler).PutPacket(p)
}

// ConnectTCP 连接TCP服务
// network: tcp, tcp4, tcp6
// addr: ip:port
func ConnectTCP(network string, addr string) (*net.TCPConn, error) {
	raddr, err := net.ResolveTCPAddr(network, addr)
	if err != nil {
		return nil, err
	}

	return net.DialTCP(network, nil, raddr)
}
