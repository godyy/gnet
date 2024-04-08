package gnet

import (
	"github.com/godyy/gutils/buffer/bytes"
)

// CustomPacket 自定义数据包
type CustomPacket interface {
	// Data 返回待发送的二进制数据
	Data() []byte
}

// RawPacket 原生数据包
type RawPacket []byte

func (p RawPacket) Data() []byte { return p }

// Packet 字节数据包，提供字节相关读写操作接口
type Packet struct {
	bytes.Buffer // 使用字节缓冲区提供基础功能
}

func (p *Packet) Data() []byte {
	return p.UnreadData()
}

func (p *Packet) ReadInt16() (int16, error) {
	return p.ReadBigInt16()
}

func (p *Packet) WriteInt16(i int16) error {
	return p.WriteBigInt16(i)
}

func (p *Packet) ReadUint16() (i uint16, err error) {
	return p.ReadBigUint16()
}

func (p *Packet) WriteUint16(i uint16) error {
	return p.WriteBigUint16(i)
}

func (p *Packet) ReadInt32() (int32, error) {
	return p.ReadBigInt32()
}

func (p *Packet) WriteInt32(i int32) error {
	return p.WriteBigInt32(i)
}

func (p *Packet) ReadUint32() (i uint32, err error) {
	return p.ReadBigUint32()
}

func (p *Packet) WriteUint32(i uint32) error {
	return p.WriteBigUint32(i)
}

func (p *Packet) ReadInt64() (int64, error) {
	return p.ReadBigInt64()
}

func (p *Packet) WriteInt64(i int64) error {
	return p.WriteBigInt64(i)
}

func (p *Packet) ReadUint64() (i uint64, err error) {
	return p.ReadBigUint64()
}

func (p *Packet) WriteUint64(i uint64) error {
	return p.WriteBigUint64(i)
}

// NewPacketWithCap 通过指定容量创建
func NewPacketWithCap(cap int) *Packet {
	if cap <= 0 {
		panic("gnet.NewBytesPacketWithCap: cap <= 0")
	}

	b := &Packet{}
	b.SetBuf(make([]byte, 0, cap))
	return b
}

// NewPacketWithSize 通过指定大小创建
func NewPacketWithSize(size int) *Packet {
	if size <= 0 {
		panic("gnet.NewBytesPacketWithCap: size <= 0")
	}

	b := &Packet{}
	b.SetBuf(make([]byte, size))
	return b
}

// NewPacket 创建Packet
// 可通过提供data来直接设置数据
func NewPacket(data []byte) *Packet {
	b := &Packet{}
	b.SetBuf(data)
	return b
}

type packetNode struct {
	p    CustomPacket
	next *packetNode
}

// PacketQueue Packet队列
type PacketQueue struct {
	head, tail *packetNode
	len        int
}

func NewPacketQueue() *PacketQueue {
	pq := &PacketQueue{}
	return pq
}

func (pq *PacketQueue) Len() int { return pq.len }

func (pq *PacketQueue) Push(p CustomPacket) {
	node := &packetNode{p: p}
	if pq.len <= 0 {
		pq.head = node
		pq.tail = node
		pq.len = 1
	} else {
		pq.tail.next = node
		pq.tail = node
		pq.len++
	}
}

func (pq *PacketQueue) Pop() CustomPacket {
	if pq.len <= 0 {
		return nil
	}

	node := pq.head
	pq.head = pq.head.next
	pq.len--
	if pq.len <= 0 {
		pq.head = nil
		pq.tail = nil
	}
	return node.p
}

func (pq *PacketQueue) Clear() {
	pq.head = nil
	pq.tail = nil
	pq.len = 0
}
