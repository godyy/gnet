package gnet

import (
	"github.com/godyy/gutils/buffer/bytes"
)

// Buffer byte 缓冲区
type Buffer struct {
	bytes.Buffer
}

func (p *Buffer) ReadInt16() (int16, error) {
	return p.ReadBigInt16()
}

func (p *Buffer) WriteInt16(i int16) error {
	return p.WriteBigInt16(i)
}

func (p *Buffer) ReadUint16() (i uint16, err error) {
	return p.ReadBigUint16()
}

func (p *Buffer) WriteUint16(i uint16) error {
	return p.WriteBigUint16(i)
}

func (p *Buffer) ReadInt32() (int32, error) {
	return p.ReadBigInt32()
}

func (p *Buffer) WriteInt32(i int32) error {
	return p.WriteBigInt32(i)
}

func (p *Buffer) ReadUint32() (i uint32, err error) {
	return p.ReadBigUint32()
}

func (p *Buffer) WriteUint32(i uint32) error {
	return p.WriteBigUint32(i)
}

func (p *Buffer) ReadInt64() (int64, error) {
	return p.ReadBigInt64()
}

func (p *Buffer) WriteInt64(i int64) error {
	return p.WriteBigInt64(i)
}

func (p *Buffer) ReadUint64() (i uint64, err error) {
	return p.ReadBigUint64()
}

func (p *Buffer) WriteUint64(i uint64) error {
	return p.WriteBigUint64(i)
}

// NewBufferWithCap 通过指定容量创建
func NewBufferWithCap(cap int) *Buffer {
	if cap <= 0 {
		panic("gnet.NewBytesPacketWithCap: cap <= 0")
	}

	b := &Buffer{}
	b.SetBuf(make([]byte, 0, cap))
	return b
}

// NewBufferWithSize 通过指定大小创建
func NewBufferWithSize(size int) *Buffer {
	if size <= 0 {
		panic("gnet.NewBytesPacketWithCap: size <= 0")
	}

	b := &Buffer{}
	b.SetBuf(make([]byte, size))
	return b
}

// NewBuffer 创建 Buffer
// 可通过提供data来直接设置数据
func NewBuffer(data []byte) *Buffer {
	b := &Buffer{}
	b.SetBuf(data)
	return b
}
