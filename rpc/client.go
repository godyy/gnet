package rpc

import (
	"encoding/binary"
	"errors"
	"github.com/godyy/gnet"
	"github.com/godyy/gutils/container/heap"
	pkg_errors "github.com/pkg/errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrTimeout 表示RPC请求超时
var ErrTimeout = errors.New("rpc: timeout")

// ClientHandler 封装RPC客户端需要实现的功能
type ClientHandler interface {
	// RPCEncodeMethodArgs 编码方法arguments.
	RPCEncodeMethodArgs(methodId uint16, args any) ([]byte, error)

	// RPCDecodeMethodReply 解码方法reply.
	RPCDecodeMethodReply(methodId uint16, replyBytes []byte) (any, error)

	// RPCEncodeRequestPacket 编码 request 数据包.
	// 根据 size 指定的大小创建请求数据包，并将以数据包缓冲区为参数调用 encoder，
	// 在 encoder 内部进行数据填充.
	RPCEncodeRequestPacket(size int, encoder func(p []byte) error) (gnet.Packet, error)
}

// Client RPC客户端
type Client struct {
	conn    Conn          // RPC连接
	handler ClientHandler // 客户端功能实现

	mutex            *sync.Mutex       // protects following.
	seq              uint64            // seq 自增键
	calls            map[uint64]*Call  // 已发起的调用
	callHeap         *heap.Heap[*Call] // 已heap形式组织的已发起调用，基于超时时间排序
	callTimeoutTimer *time.Timer       // 超时timer，用于设置已发起调用的最早超时时间
	close            bool              // 是否关闭
}

// NewClient 创建RPC Client
func NewClient(conn Conn, h ClientHandler) *Client {
	if h == nil {
		panic("rpc.NewClient: client handler nil")
	}

	cli := &Client{
		conn:     conn,
		handler:  h,
		mutex:    new(sync.Mutex),
		seq:      0,
		calls:    make(map[uint64]*Call),
		callHeap: heap.NewHeap[*Call](),
	}

	return cli
}

// addCall 添加调用
func (c *Client) addCall(call *Call) {
	if _, ok := c.calls[call.seq]; ok {
		panic("rpc: add duplicate call")
	}
	c.calls[call.seq] = call

	c.callHeap.Push(call)
	if topCall := c.callHeap.Top(); topCall == call {
		c.resetCallTimeout(call.timeout)
	}
}

// remCall 移除调用
func (c *Client) remCall(call *Call) {
	top := c.callHeap.Top() == call
	c.callHeap.Remove(call.HeapIndex())
	delete(c.calls, call.seq)
	if top {
		if c.callHeap.Len() > 0 {
			c.resetCallTimeout(c.callHeap.Top().timeout)
		} else {
			c.stopCallTimeout()
		}
	}
}

// topCall 返回当前堆顶的调用
func (c *Client) topCall() *Call {
	if c.callHeap.Len() <= 0 {
		return nil
	}
	return c.callHeap.Top()
}

// popCall 获取并移除id指定的调用
func (c *Client) popCall(seq uint64) *Call {
	call := c.calls[seq]
	if call == nil {
		return nil
	}

	c.remCall(call)
	return call
}

// resetCallTimeout 重置调用超时
func (c *Client) resetCallTimeout(timeoutMs int64) {
	d := time.Duration(timeoutMs-time.Now().UnixMilli()) * time.Millisecond
	if c.callTimeoutTimer == nil {
		c.callTimeoutTimer = time.AfterFunc(d, c.onCallTimeout)
	} else {
		c.callTimeoutTimer.Reset(d)
	}
}

// stopCallTimeout 停止调用超时
func (c *Client) stopCallTimeout() {
	if c.callTimeoutTimer == nil {
		return
	}
	c.callTimeoutTimer.Stop()
}

// onCallTimeout 调用超时回调
func (c *Client) onCallTimeout() {
	for {
		c.mutex.Lock()

		// 取出堆顶调用，若堆已空，退出循环
		call := c.topCall()
		if call == nil {
			c.mutex.Unlock()
			break
		}

		// 若调用未超时，重置超时定时器，退出循环
		if !call.isTimeout(time.Now().UnixMilli()) {
			c.resetCallTimeout(call.timeout)
			c.mutex.Unlock()
			break
		}

		// 移除调用
		c.remCall(call)

		c.mutex.Unlock()

		// 报告调用超时
		call.onError(ErrTimeout)
	}
}

// sendCall 调用请求发送逻辑
// conn 表示PRC连接, methodId 和 args 分别为调用的方法ID和参数，
// callback 用于指定调用完成时触发的回调函数，timeout 表示调用超时时间，单位为毫秒.
func (c *Client) sendCall(methodId uint16, args any, timeout int64, callback Callback) error {
	if args == nil {
		return errors.New("rpc: args nil")
	}

	if callback == nil {
		return errors.New("rpc: callback nil")
	}

	if timeout <= 0 {
		return ErrTimeout
	}

	call := &Call{
		MethodId:  methodId,
		Args:      args,
		timeout:   time.Now().UnixMilli() + timeout,
		heapIndex: -1,
		callback:  callback,
	}

	// 获取 seq.
	c.mutex.Lock()
	if c.close {
		c.mutex.Unlock()
		return ErrShutdown
	}
	call.seq = c.seq
	c.seq++
	c.mutex.Unlock()

	// 编码参数 arguments.
	argsBytes, err := c.handler.RPCEncodeMethodArgs(methodId, args)
	if err != nil {
		return pkg_errors.WithMessagef(err, "rpc: encode method:%d args", methodId)
	}

	// 编码 request.
	encoder := requestPacketEncoder{
		seq:      call.seq,
		methodId: methodId,
		args:     argsBytes,
	}
	p, err := c.handler.RPCEncodeRequestPacket(encoder.size(), encoder.encode)
	if err != nil {
		return pkg_errors.WithMessage(err, "rpc: encode request packet")
	}

	// 创建并添加调用
	c.mutex.Lock()
	if c.close {
		c.mutex.Unlock()
		return ErrShutdown
	}
	c.addCall(call)
	c.mutex.Unlock()

	// 发送请求数据包
	if err = c.conn.WritePacket(p); err != nil {
		c.mutex.Lock()
		c.remCall(call)
		c.mutex.Unlock()
		return pkg_errors.WithMessage(err, "rpc: send request packet")
	}

	return nil
}

// AsyncCall 异步的RPC调用，调用完成时会触发回调函数 callback.
func (c *Client) AsyncCall(methodId uint16, args any, timeout int64, callback Callback) error {
	return c.sendCall(methodId, args, timeout, callback)
}

// Call 同步的RPC调用.
func (c *Client) Call(methodId uint16, args any, timeout int64) (any, error) {
	callback := syncDoneCallback{done: make(chan struct{}, 1)}
	if err := c.AsyncCall(methodId, args, timeout, callback.onCallback); err != nil {
		close(callback.done)
		return nil, err
	}
	<-callback.done
	return callback.reply, callback.err
}

// ErrResponseTooShort 表示 response 数据长度过短
var ErrResponseTooShort = errors.New("rpc: response data too short")

// ErrResponseMethodId 表示 response 中的方法ID错误
var ErrResponseMethodId = errors.New("rpc: response with wrong method id")

// HandleResponse 处理RPC响应. data 为响应数据包的二进制流.
func (c *Client) HandleResponse(data []byte) error {
	if len(data) <= responseMinLen {
		return ErrResponseTooShort
	}

	// 解码 seq
	seq := binary.BigEndian.Uint64(data[:seqLen])

	// 匹配调用
	c.mutex.Lock()
	if c.close {
		c.mutex.Unlock()
		return ErrShutdown
	}
	call := c.popCall(seq)
	if call == nil {
		// Call already timeout
		c.mutex.Unlock()
		return nil
	}
	c.mutex.Unlock()

	// 解码方法ID, 并核对方法ID
	methodId := binary.BigEndian.Uint16(data[seqLen:seqMethodIdLen])
	if call.MethodId != methodId {
		call.onError(ErrResponseMethodId)
		return nil
	}

	// 解码 error flag
	errorFlag := data[seqMethodIdLen] != 0

	if errorFlag {
		call.onError(errors.New(string(data[seqMethodIdLen+1:])))
	} else {
		if reply, err := c.handler.RPCDecodeMethodReply(methodId, data[seqMethodIdLen+1:]); err != nil {
			call.onError(pkg_errors.WithMessagef(err, "rpc: decode method:%d reply", methodId))
		} else {
			call.onReply(reply)
		}
	}

	return nil
}

// Close 关闭客户端，同时为正在执行的调用返回 ErrShutdown.
func (c *Client) Close() error {
	c.mutex.Lock()
	if c.close {
		c.mutex.Unlock()
		return ErrShutdown
	}
	c.stopCallTimeout()
	for _, call := range c.calls {
		call.onError(ErrShutdown)
	}
	c.close = true
	c.mutex.Unlock()
	return nil
}

// Callback RPC调用完成回调.
type Callback func(reply any, err error)

// Call 表示一个正在进行的RPC调用.
type Call struct {
	MethodId uint16 // 调用目标方法ID
	Args     any    // 参数
	Reply    any    // 返回值
	Error    error  // error

	seq       uint64   // 序号
	timeout   int64    // 超时，毫秒
	heapIndex int      // 堆索引
	doneFlag  int32    // done flag
	callback  Callback // callback
}

func (c *Call) HeapLess(element heap.Element) bool {
	othCall := element.(*Call)
	return c.timeout < othCall.timeout
}

func (c *Call) SetHeapIndex(i int) {
	c.heapIndex = i
}

func (c *Call) HeapIndex() int {
	return c.heapIndex
}

func (c *Call) isTimeout(nowMilli int64) bool {
	return c.timeout <= nowMilli
}

func (c *Call) invokeCallback() {
	c.callback(c.Reply, c.Error)
}

func (c *Call) done() bool {
	return atomic.CompareAndSwapInt32(&c.doneFlag, 0, 1)
}

func (c *Call) onReply(reply any) {
	if c.done() {
		c.Reply = reply
		c.invokeCallback()
	}
}

func (c *Call) onError(err error) {
	if c.done() {
		c.Error = err
		c.invokeCallback()
	}
}

// syncDoneCallback 同步RPC调用回调封装
type syncDoneCallback struct {
	done  chan struct{}
	reply any
	err   error
}

func (c *syncDoneCallback) onCallback(reply any, err error) {
	c.reply = reply
	c.err = err
	close(c.done)
}
