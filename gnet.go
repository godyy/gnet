package gnet

import (
	"io"
	"net"
	"time"
)

// ConnReader 连接Reader
type ConnReader interface {
	SetReadDeadline(t time.Time) error
	io.Reader
}

// ConnWriter 连接Writer
type ConnWriter interface {
	SetWriteDeadline(t time.Time) error
	io.Writer
}

// ConnReaderFrom 连接ReaderFrom
type ConnReaderFrom interface {
	SetReadDeadline(t time.Time) error
	ReadFrom([]byte) (int, net.Addr, error)
}

// ConnWriterTo 连接WriterTo
type ConnWriterTo interface {
	SetWriteDeadline(t time.Time) error
	WriteTo([]byte, net.Addr) (int, error)
}
