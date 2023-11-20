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

// PacketQueue Packet队列
type PacketQueue struct {
	list *list.List
}

func NewPacketQueue() *PacketQueue {
	return &PacketQueue{
		list: list.New(),
	}
}

func (pq *PacketQueue) Len() int { return pq.list.Len() }

func (pq *PacketQueue) Push(p *Packet) {
	pq.list.PushBack(p)
}

func (pq *PacketQueue) Pop() *Packet {
	if pq.list.Len() <= 0 {
		return nil
	}
	return pq.list.Remove(pq.list.Front()).(*Packet)
}

func (pq *PacketQueue) Clear() {
	pq.list.Init()
}
