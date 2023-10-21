package gnet

import (
	"github.com/godyy/gutils/buffer/bytes"
	pkg_errors "github.com/pkg/errors"
)

// SendBuffer 实现了读写分离的发送缓冲区
type SendBuffer struct {
	writer *bytes.Buffer
	reader *bytes.Buffer
}

func NewSendBuffer(cap ...int) *SendBuffer {
	return &SendBuffer{
		writer: bytes.NewBuffer(cap...),
		reader: bytes.NewBuffer(cap...),
	}
}

func (b *SendBuffer) Writer() *bytes.Buffer {
	return b.writer
}

func (b *SendBuffer) Reader() *bytes.Buffer {
	return b.reader
}

// WritePacket 将数据包的内容填充入缓冲区
func (b *SendBuffer) WritePacket(p *Packet) error {
	if err := b.writer.WriteUint32(uint32(p.Readable())); err != nil {
		return pkg_errors.WithMessage(err, "write packet size")
	}
	if _, err := b.writer.Write(p.UnreadData()); err != nil {
		return pkg_errors.WithMessage(err, "write packet data")
	}
	return nil
}

// Swap 缓冲区读写交换
func (b *SendBuffer) Swap(minCap, maxCap int) {
	if b.reader.Readable() > 0 {
		panic("reader not empty")
	}
	b.reader.ResetCap(minCap, maxCap)
	b.writer, b.reader = b.reader, b.writer
}
