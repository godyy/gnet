package gnet

import (
	"net"
)

// Packet 数据包
type Packet interface {
	Data() []byte
}

// PacketReader 接收数据包
type PacketReader interface {
	SessionReadPacket(r ConnReader) (Packet, error)
}

// ReadPacket 利用 r 从 conn 指定的连接中接收数据包
func ReadPacket(conn net.Conn, r PacketReader) (Packet, error) {
	return r.SessionReadPacket(conn)
}

// PacketWriter 发送数据包
type PacketWriter interface {
	WritePacket(w ConnWriter, p Packet) error
}

// WritePacket 利用 w 将数据包 p 通过 conn 指定的连接发送出去
func WritePacket(conn net.Conn, p Packet, w PacketWriter) error {
	return w.WritePacket(conn, p)
}

// PacketReaderFrom 接收数据包并返回发送方地址
type PacketReaderFrom interface {
	ReadPacketFrom(r ConnReaderFrom) (Packet, net.Addr, error)
}

// ReadPacketFrom 利用 r 从 conn 指定的连接中接收数据包，并返回发送方地址
func ReadPacketFrom(conn net.PacketConn, r PacketReaderFrom) (Packet, net.Addr, error) {
	return r.ReadPacketFrom(conn)
}

// PacketWriterTo 将数据包发送到指定地址
type PacketWriterTo interface {
	WritePacketTo(w ConnWriterTo, p Packet, addr net.Addr) error
}

// WritePacketTo 利用 w 将数据包 p 通过 conn 指定的连接发送到 addr 指定的地址
func WritePacketTo(conn net.PacketConn, p Packet, addr net.Addr, w PacketWriterTo) error {
	return w.WritePacketTo(conn, p, addr)
}
