package gnet

import (
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
