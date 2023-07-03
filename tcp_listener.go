package gnet

import (
	"net"
)

// TCPListener TCP连接监听器
// 负责监听TCP连接请求，并创建TCPSession
type TCPListener struct {
	listener *net.TCPListener
}

// Start 开始监听连接
// Start内部会一直保持阻塞状态监听连接请求，直到有新的请求到来，或者TCPListener被关闭，或者出现错误
func (l *TCPListener) Start(onNewConn func(net.Conn)) error {
	if onNewConn == nil {
		panic("gnet.TCPListener.Start: onNewConn nil")
	}

	var tcpConn *net.TCPConn
	var err error
	for {
		tcpConn, err = l.listener.AcceptTCP()
		if err != nil {
			return err
		}
		onNewConn(tcpConn)
	}
}

// Close 关闭监听器
// 关闭监听器，停止接受连接请求
func (l *TCPListener) Close() error {
	return l.listener.Close()
}

// ListenTCP 创建TCPListener
// network: tcp, tcp4, tcp6
// addr: ip:port
func ListenTCP(network string, addr string) (*TCPListener, error) {
	var err error

	var tcpAddr *net.TCPAddr
	tcpAddr, err = net.ResolveTCPAddr(network, addr)
	if err != nil {
		return nil, err
	}

	var tcpListener *net.TCPListener
	tcpListener, err = net.ListenTCP(network, tcpAddr)
	if err != nil {
		return nil, err
	}

	return &TCPListener{listener: tcpListener}, nil
}
