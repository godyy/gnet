# Introduction
Network library implemented based on the Go standard library net package.

# Examples
TCP
```go
// Define session handler, implement gnet.SessionHandler
type SessionHandler struct {}

func (h *SessionHandler) OnSessionPacket(gnet.Session, *gnet.Packet) {
	fmt.Println("OnSessionPacket")
}

func (h *SessionHandler) OnSessionClose(gnet.Session, error) {
	fmt.Println("OnSessionClose")
}

// Create and set session options.
opt := gnet.NewTCPSessionOption()
...

// Create session handler.
sessionHandler := &SessionHandler{}

// Create tcp listener.
listener, err := gnet.ListenTCP("tcp", "localhost:8822")
...

// Start listening.
err = listener.Start(func (conn *net.TPCConn) {
	// You can do something before starting session at here.
	...
	
	// Create and start tcp session.
	session := gnet.NewTCPSession(conn)
	err := session.Start(opt, sessionHandler)
	...
})
...
```