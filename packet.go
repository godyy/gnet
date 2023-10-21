package gnet

import (
	"container/list"
	"sync"

	"github.com/godyy/gutils/buffer/bytes"
)

// Packet 网络消息包
type Packet struct {
	*bytes.Buffer // 使用字节缓冲区提供基础功能
}

// NewPacket 通过指定容量创建消息包
func NewPacket(cap ...int) *Packet {
	return &Packet{Buffer: bytes.NewBuffer(cap...)}
}

// NewPacketWithData 创建Packet
// 可通过提供data来直接设置数据
func NewPacketWithData(data ...[]byte) *Packet {
	return &Packet{
		Buffer: bytes.NewBufferWithBuf(data...),
	}
}

// packet公共对象池
var packetPool = &sync.Pool{
	New: func() interface{} {
		p := NewPacket()
		return p
	},
}

// GetPacket 从公共对象池中获取Packet
func GetPacket(cap ...int) *Packet {
	p := packetPool.Get().(*Packet)
	if len(cap) > 0 {
		p.Grow(cap[0])
	}
	return p
}

// PutPacket 将Packet回收到公共对象池中
func PutPacket(p *Packet) {
	p.Reset()
	packetPool.Put(p)
}

// PendingPacketQueue 读写分离的待发送数据包队列
type PendingPacketQueue struct {
	in  *list.List
	out *list.List
}

func NewPendingPacketQueue() *PendingPacketQueue {
	return &PendingPacketQueue{
		in:  list.New(),
		out: list.New(),
	}
}

func (q *PendingPacketQueue) Available() bool {
	if q.in.Len() <= 0 {
		return false
	}

	if q.out.Len() > 0 {
		panic("gnet.PendingPacketQueue: there are packets not popped")
	}

	q.in, q.out = q.out, q.in
	return true
}

func (q *PendingPacketQueue) Push(p *Packet) {
	q.in.PushBack(p)
}

func (q *PendingPacketQueue) Pop() *Packet {
	front := q.out.Front()
	if front == nil {
		return nil
	}
	return q.out.Remove(front).(*Packet)
}

func (q *PendingPacketQueue) Clear() {
	q.in.Init()
	q.out.Init()
}
