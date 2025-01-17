package define

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/godyy/gnet"
	"github.com/godyy/gutils/buffer"
	"github.com/godyy/gutils/buffer/bytes"
	pkg_errors "github.com/pkg/errors"
	"io"
	"sync"
	"time"
)

var bufferPool = &sync.Pool{
	New: func() any {
		return gnet.NewBuffer(nil)
	},
}

func GetBuffer() *gnet.Buffer {
	return bufferPool.Get().(*gnet.Buffer)
}

func PutBuffer(p *gnet.Buffer) {
	p.Reset()
	bufferPool.Put(p)
}

type PacketReaderWriter struct {
	readBuffer    *bytes.FixedBuffer
	writeBuffer   *bytes.FixedBuffer
	maxPacketSize int
}

func NewPacketReaderWriter(readBufferSize, writeBufferSize, maxPacketSize int) *PacketReaderWriter {
	return &PacketReaderWriter{
		readBuffer:    bytes.NewFixedBuffer(readBufferSize),
		writeBuffer:   bytes.NewFixedBuffer(writeBufferSize),
		maxPacketSize: maxPacketSize,
	}
}

func (rw *PacketReaderWriter) readToBuffer(r gnet.ConnReader) error {
	if err := r.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return pkg_errors.WithMessage(err, "set read deadline")
	}
	_, err := rw.readBuffer.ReadFrom(r)
	return err
}

func (rw *PacketReaderWriter) read(r gnet.ConnReader, b []byte) error {
	var (
		rr int
	)
	for rr < len(b) {
		n, err := rw.readBuffer.Read(b[rr:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				if err := rw.readToBuffer(r); err != nil {
					return err
				}
				continue
			}
			return err
		}
		rr += n
	}
	return nil
}

func (rw *PacketReaderWriter) SessionReadPacket(r gnet.ConnReader) (gnet.Packet, error) {
	// 读取数据包大小
	var head [4]byte
	if err := rw.read(r, head[:]); err != nil {
		return nil, pkg_errors.WithMessage(err, "read packet size")
	}
	size := binary.BigEndian.Uint32(head[:])
	if size > uint32(rw.maxPacketSize) {
		return nil, fmt.Errorf("packet size %d overflow", size)
	}

	// 读取
	p := GetBuffer()
	p.Grow(int(size), true)
	if err := rw.read(r, p.Data()); err != nil {
		return nil, pkg_errors.WithMessage(err, "read packet data")
	}

	return p, nil
}

func (rw *PacketReaderWriter) writeFromBuffer(w gnet.ConnWriter) error {
	if err := w.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return pkg_errors.WithMessage(err, "set write deadline")
	}
	for rw.writeBuffer.Readable() > 0 {
		if _, err := rw.writeBuffer.WriteTo(w); err != nil {
			return err
		}
	}
	return nil
}

func (rw *PacketReaderWriter) write(w gnet.ConnWriter, b []byte) error {
	var (
		ww int
	)
	for ww < len(b) {
		n, err := rw.writeBuffer.Write(b[ww:])
		if err != nil {
			if errors.Is(err, buffer.ErrBufferFull) {
				if err := rw.writeFromBuffer(w); err != nil {
					return err
				}
				continue
			}
			return err
		}
		ww += n
	}
	return nil
}

func (rw *PacketReaderWriter) SessionWritePacket(w gnet.ConnWriter, p gnet.Packet, more bool) error {
	// 写入数据包大小
	size := len(p.Data())
	if size > int(rw.maxPacketSize) {
		return fmt.Errorf("packet size %d overflow", size)
	}
	head := make([]byte, 4)
	binary.BigEndian.PutUint32(head, uint32(size))
	if err := rw.write(w, head[:]); err != nil {
		return pkg_errors.WithMessage(err, "write packet size")
	}

	// 写入数据
	if err := rw.write(w, p.Data()); err != nil {
		return pkg_errors.WithMessage(err, "write packet data")
	}

	PutBuffer(p.(*gnet.Buffer))

	if !more && rw.writeBuffer.Readable() > 0 {
		if err := rw.writeFromBuffer(w); err != nil {
			return err
		}
	}

	return nil
}
