package gnet

import (
	"fmt"
	"io"
	"math"
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

	// tcp消息包包体理论最大大小
	tcpMaxPacketSize = math.MaxUint32

	// tcp会话默认接收缓冲区大小
	tcpDefaultReceiveBufferSize = 4096

	// tcp会话默认发送缓冲区大小
	tcpDefaultSendBufferSize = 4096
)

// TCPSessionOption TCP会话选项
type TCPSessionOption struct {
	locker sync.Mutex

	// refCount 引用计数
	// 值大于0时，表示被占用，无法修改属性。
	refCount int

	// receiveTimeout 接收超时
	// default zero, mean no limit.
	receiveTimeout time.Duration

	// sendTimeout 发送超时
	// default zero, mean no limit.
	sendTimeout time.Duration

	// receiveBufferSize 接收缓冲区大小
	// default 4096.
	receiveBufferSize int

	// sendBufferSize 发送缓冲区大小
	// default 4096.
	sendBufferSize int

	// maxPacketSize 最大消息包大小
	// 接收到超过此上限的消息包，会默认连接非法.
	// 理论上限为 MaxUint32，default 4092 = 4096 - 4.
	maxPacketSize int
}

// NewTCPSessionOption 创建默认TCP会话选项
func NewTCPSessionOption() *TCPSessionOption {
	return &TCPSessionOption{
		receiveTimeout:    0,
		sendTimeout:       0,
		receiveBufferSize: tcpDefaultReceiveBufferSize,
		sendBufferSize:    tcpDefaultSendBufferSize,
		maxPacketSize:     tcpDefaultReceiveBufferSize - tcpPacketSizeLen,
	}
}

// acquire 占用，增加引用计数。
// 当引用计数>0时，表示选项正在被使用。当选项被占用后，选项的相关属性值不得再更改。
func (o *TCPSessionOption) acquire() *TCPSessionOption {
	o.locker.Lock()
	defer o.locker.Unlock()
	o.refCount++
	return o
}

// release 释放，降低引用计数。
// 当引用计数==0时，表示选项没有被占用，可以修改选项相关的属性值。
func (o *TCPSessionOption) release() {
	o.locker.Lock()
	defer o.locker.Unlock()
	if o.refCount > 0 {
		o.refCount--
	}
}

// lock 锁定
// 若选项未被占用，加锁成功，可以对选项属性值进行修改。反之，锁定失败。
func (o *TCPSessionOption) lock() bool {
	o.locker.Lock()
	if o.refCount > 0 {
		o.locker.Unlock()
		return false
	}
	return true
}

// unlock 解除锁定
// 修改完成后，解除对选项的锁定。
func (o *TCPSessionOption) unlock() {
	o.locker.Unlock()
}

// SetReceiveTimeout 设置接收超时
func (o *TCPSessionOption) SetReceiveTimeout(timeout time.Duration) *TCPSessionOption {
	if timeout < 0 {
		panic("gnet.TCPSessionOption.SetReceiveTimeout: timeout < 0")
	}

	if !o.lock() {
		return o
	}
	defer o.unlock()

	o.receiveTimeout = timeout
	return o
}

// SetSendTimeout 设置发送超时
func (o *TCPSessionOption) SetSendTimeout(timeout time.Duration) *TCPSessionOption {
	if timeout < 0 {
		panic("gnet.TCPSessionOption.SetSendTimeout: timeout < 0")
	}

	if !o.lock() {
		return o
	}
	defer o.unlock()

	o.sendTimeout = timeout
	return o
}

// SetReceiveBufferSize 设置接收缓冲区大小
func (o *TCPSessionOption) SetReceiveBufferSize(s int) *TCPSessionOption {
	if s <= 0 {
		panic("gnet.TCPSessionOption.SetReceiveBufferSize: size <= 0")
	}

	if !o.lock() {
		return o
	}
	defer o.unlock()

	o.receiveBufferSize = s
	return o
}

// SetSendBufferSize 设置发送缓冲区大小
func (o *TCPSessionOption) SetSendBufferSize(s int) *TCPSessionOption {
	if s <= 0 {
		panic("gnet.TCPSessionOption.SetSendBufferSize: size <= 0")
	}

	if !o.lock() {
		return o
	}
	defer o.unlock()

	o.sendBufferSize = s
	return o
}

// SetMaxPacketSize 设置最大消息包大小
// 接收到超过此上限的消息包，会默认连接非法.
func (o *TCPSessionOption) SetMaxPacketSize(s int) *TCPSessionOption {
	if s <= 0 || s > tcpMaxPacketSize {
		panic("gnet.TCPSessionOption.SetMaxPacketSize: size out of range")
	}

	if !o.lock() {
		return o
	}
	defer o.unlock()

	o.maxPacketSize = s
	return o
}

// GetReceiveTimeout 获取接收超时
func (o *TCPSessionOption) GetReceiveTimeout() time.Duration { return o.receiveTimeout }

// GetSendTimeout 获取发送超时
func (o *TCPSessionOption) GetSendTimeout() time.Duration { return o.sendTimeout }

// GetReceiveBufferSize 获取接收缓冲区大小
func (o *TCPSessionOption) GetReceiveBufferSize() int { return o.receiveBufferSize }

// GetSendBufferMinSize 获取最小发送缓冲区大小
func (o *TCPSessionOption) GetSendBufferMinSize() int { return o.sendBufferSize }

// GetMaxPacketSize 获取最大消息包大小
func (o *TCPSessionOption) GetMaxPacketSize() int { return o.maxPacketSize }

// TCPSession TCP网络会话
type TCPSession struct {
	locker            sync.RWMutex       // locker
	state             int32              // 会话状态
	conn              *net.TCPConn       // tcp conn
	opt               *TCPSessionOption  // 会话选项，用于读取会话设置
	handler           SessionHandler     // 会话处理器
	pendingPacketCond *sync.Cond         // 发送条件信号
	pendingPacketQue  *PacketQueue       // 待发送数据包队列
	sendBuf           *bytes.FixedBuffer // 发送缓冲区
	receiveBuf        *bytes.FixedBuffer // 接收缓冲区
	closeTag          int32              // 关闭标记
	closeErr          *error             // 造成会话关闭的error
}

// NewTCPSession 创建TCPSession
// conn 为底层TCP连接，opt 提供TCPSession使用的选项参数，h 为会话事件处理处理器
func NewTCPSession(conn *net.TCPConn) *TCPSession {
	if conn == nil {
		panic("gnet.NewTCPSession: conn nil")
	}

	s := &TCPSession{
		state: SessionIdle,
		conn:  conn,
	}
	return s
}

// Start 根据opt提供的选项参数启动后台goroutine，使会话进入SessionRunning状态。
// 通过h反馈会话事件。
func (s *TCPSession) Start(opt *TCPSessionOption, h SessionHandler) error {
	if opt == nil {
		panic("gnet.TCPSession.Start: opt nil")
	}

	if h == nil {
		panic("gnet.TCPSession.Start: h nil")
	}

	s.locker.Lock()
	defer s.locker.Unlock()

	if err := s.checkState(SessionIdle); err != nil {
		return err
	}

	s.opt = opt.acquire()
	s.handler = h
	s.pendingPacketCond = sync.NewCond(&s.locker)
	s.pendingPacketQue = NewPacketQueue()
	atomic.StoreInt32(&s.state, SessionStarted)
	go s.receiveLoop()
	go s.sendLoop()
	return nil
}

// State 获取会话当前所处状态。
func (s *TCPSession) State() int32 {
	return atomic.LoadInt32(&s.state)
}

func (s *TCPSession) checkState(state int32) error {
	curState := atomic.LoadInt32(&s.state)
	if curState == state {
		return nil
	}

	switch curState {
	case SessionIdle:
		return ErrSessionNotStarted
	case SessionStarted:
		return ErrSessionStarted
	case SessionClosed:
		return ErrSessionClosed
	default:
		panic(fmt.Sprintf("gnet.TCPSession: illegal session state %d", curState))
	}
}

// LocalAddr 本会本地地址
func (s *TCPSession) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

// RemoteAddr 返回对端地址
func (s *TCPSession) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

// Handler 获取handler
func (s *TCPSession) Handler() SessionHandler {
	s.locker.RLocker()
	defer s.locker.RUnlock()
	return s.handler
}

// SendPacket 发送数据包
// p 所提供的消息包不会立即发送，而是被放入发送队列中，等待发送goroutine提取并发送。
// 如果 p 的大小超过 opt.GetMaxPacketSize() 或为0，返回 ErrPacketSizeOutOfRange。
// 如果会话已经关闭，返回 ErrSessionClosed。如果发送队列长度已经达到 opt.GetSendQueueSize()
// 返回 ErrSendQueueFull。
func (s *TCPSession) SendPacket(p *Packet) error {
	if p == nil {
		panic("gnet.TCPSession.SendPacket: p nil")
	}

	s.locker.Lock()
	defer s.locker.Unlock()

	if err := s.checkState(SessionStarted); err != nil {
		return err
	}

	if size := p.Readable(); size == 0 || size > s.opt.GetMaxPacketSize() {
		return ErrPacketSizeOutOfRange
	}

	s.pendingPacketQue.Push(p)
	s.pendingPacketCond.Signal()
	return nil
}

// Close 关闭会话
// reason 用于提供关闭会话的原因，该 reason 在会话关闭后，可以通过 CloseErr() 获取，
// 但前提是调用 Close(reason) 之前，会话并未关闭.
func (s *TCPSession) Close(reason error) error {
	return s.close(true, reason)
}

// close 关闭会话的实际实现
func (s *TCPSession) close(active bool, err error) error {
	if active {
		if atomic.LoadInt32(&s.state) == SessionIdle {
			return ErrSessionNotStarted
		}
	}

	ok := atomic.CompareAndSwapInt32(&s.state, SessionStarted, SessionClosed)
	if ok {
		s.setCLoseErr(err)
		_ = s.conn.Close()
		s.pendingPacketCond.Signal()
	}

	if active {
		if !ok {
			return ErrSessionClosed
		}
		return nil
	} else {
		s.setCLoseErr(err)
		ct := atomic.AddInt32(&s.closeTag, 1)
		if ct == 2 {
			s.handler.OnSessionClosed(s, s.getCloseErr())
			s.locker.Lock()
			s.handler = nil
			s.locker.Unlock()
			s.opt.release()
			s.pendingPacketQue.Clear()
		}
		return nil
	}
}

func (s *TCPSession) setCLoseErr(err error) {
	atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.closeErr)), unsafe.Pointer(nil), unsafe.Pointer(&err))
}

func (s *TCPSession) getCloseErr() error {
	return *(*error)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.closeErr))))
}

// sendLoop 发送循环
// sendLoop 在发送数据出错，或会话关闭后，会自动退出。
func (s *TCPSession) sendLoop() {
	var err error

	s.sendBuf = bytes.NewFixedBuffer(s.opt.GetSendBufferMinSize())

	for err = s.checkState(SessionStarted); err == nil; err = s.checkState(SessionStarted) {
		pq := s.swapPendingPacketQue()
		if err = s.sendPacketQueue(pq); err != nil {
			break
		}
	}

	// 尽量保证缓存数据能发送出去
	_ = s.sendBuffered(true)
	s.sendBuf = nil

	// 发送出错，关闭会话
	_ = s.close(false, err)
}

// swapPendingPacketQue 置换待发送消息队列
func (s *TCPSession) swapPendingPacketQue() *PacketQueue {
	s.locker.Lock()
	defer s.locker.Unlock()
	for {
		if s.pendingPacketQue.Len() > 0 {
			pq := s.pendingPacketQue
			s.pendingPacketQue = NewPacketQueue()
			return pq
		}

		if s.checkState(SessionStarted) != nil {
			return nil
		}

		s.pendingPacketCond.Wait()
	}
}

// sendPacketQueue 发送消息包队列
func (s *TCPSession) sendPacketQueue(pq *PacketQueue) (err error) {
	if pq == nil {
		return nil
	}

	for p := pq.Pop(); p != nil; p = pq.Pop() {
		if err = s.writePacket(p); err != nil {
			return
		}
		PutPacket(p)
	}

	if err = s.sendBuffered(true); err != nil {
		err = pkg_errors.WithMessage(err, "gnet.TCPSession: send data buffered")
	}

	return
}

// writePacket 将数据包写入发送缓冲区
func (s *TCPSession) writePacket(p *Packet) error {
	var err error

	// 写包大小
	for s.sendBuf.Writable() < tcpPacketSizeLen {
		if err = s.sendBuffered(false); err != nil {
			return pkg_errors.WithMessage(err, "gnet.TCPSession: write packet size: send data buffered")
		}
	}
	err = s.sendBuf.WriteUint32(uint32(p.Readable()))
	if err != nil {
		// 将包大小写入缓存失败
		return pkg_errors.WithMessage(err, "gnet.TCPSession: write packet size to buffer")
	}

	// 写包体
	var n int
	var w int
	data := p.UnreadData()
	for n < len(data) {
		w, err = s.sendBuf.Write(data[n:])
		if err != nil && err != buffer.ErrBufferFull {
			// 将包数据写入缓存失败
			return pkg_errors.WithMessage(err, "gnet.TCPSession: write packet data to buffer")
		}
		n += w

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
		if sendTimeout := s.opt.GetSendTimeout(); sendTimeout > 0 {
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
// 再生成消息包。接收缓冲区的大小由 opt.GetReceiveBufferSize 提供。
// receiveLoop 在接收出错或会话关闭后会自动退出。
func (s *TCPSession) receiveLoop() {
	var err error

	// 申请接收缓冲区
	s.receiveBuf = bytes.NewFixedBuffer(s.opt.GetReceiveBufferSize())

	for err = s.checkState(SessionStarted); err == nil; err = s.checkState(SessionStarted) {
		if err = s.receivePacket(); err != nil {
			break
		}
	}

	s.receiveBuf = nil
	_ = s.close(false, err)
}

// receivePacket 接收单个消息包
// 事先将数据读入接收缓冲区中，再通过接收缓冲区中的数据生成消息包，并放入接收队列.
// closed 用于返回会话是否关闭，err 用于返回接收过程中产生的错误。
func (s *TCPSession) receivePacket() (err error) {
	var packetSize int

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
		packetSize = int(u32)
		if packetSize == 0 || packetSize > s.opt.GetMaxPacketSize() {
			// 包大小超出范围
			err = fmt.Errorf("gnet.TCPSession: receive packet size %d out of range", packetSize)
			return
		}
	}

	packet := GetPacket(packetSize)
	unread := packetSize
	n := int64(0)
	for {
		n, err = packet.ReadFromN(s.receiveBuf, unread)
		if err != nil && err != io.EOF {
			err = pkg_errors.WithMessage(err, "gnet.TCPSession: read data from buffer")
			return
		}

		if unread -= int(n); unread <= 0 {
			break
		}

		if err = s.receive2Buffer(); err != nil {
			err = pkg_errors.WithMessage(err, "gnet.TCPSession: receive data to buffer")
			return
		}
	}

	err = s.handler.OnSessionPacket(s, packet)
	return
}

// receive2Buffer 将数据读入接收缓冲区
// err 用于返回读取过程中出现的错误
func (s *TCPSession) receive2Buffer() (err error) {
	// set read timeout
	if receiveTimeout := s.opt.GetReceiveTimeout(); receiveTimeout > 0 {
		s.conn.SetReadDeadline(time.Now().Add(receiveTimeout))
	} else {
		s.conn.SetReadDeadline(time.Time{})
	}

	// 读取数据并放入缓存
	_, err = s.receiveBuf.ReadFrom(s.conn)
	return
}

// ConnectTCP 连接TCP服务并创建TCPSession
// network: tcp, tcp4, tcp6
// addr: ip:port
func ConnectTCP(network string, addr string) (*TCPSession, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	tcpConn := conn.(*net.TCPConn)
	return NewTCPSession(tcpConn), nil
}
