package main

import (
	"flag"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/godyy/gnet"
	"github.com/godyy/gnet/internal/examples/chat"
	client2 "github.com/godyy/gnet/internal/examples/chat/client"
	"github.com/godyy/gnet/internal/examples/chat/protocol"
	"github.com/pkg/errors"
	"log"
	"sync"
	"time"
)

var (
	ErrTerminated = errors.New("terminated")

	serverAddr = flag.String("server-addr", "localhost:8822", "specify server address")

	opt = gnet.NewTCPSessionOption()
)

func main() {
	flag.Parse()

	opt.SetReceiveTimeout(chat.ReceiveTimeout)
	opt.SetSendTimeout(chat.SendTimeout)

	client := newClient()
	if err := client.loop(); err != nil {
		log.Fatal(err)
	}
}

const (
	stateLogin = 1 // 登陆状态
	stateChat  = 2 // 聊天状态
)

type client struct {
	mtx      sync.Mutex
	state    int32
	client   *client2.Client
	readLine *readline.Instance
	userName string
}

func newClient() *client {
	c := &client{
		state: stateLogin,
	}

	readLine, err := readline.New("")
	if err != nil {
		panic(errors.WithMessage(err, "内部错误 readline"))
	}

	c.readLine = readLine
	return c
}

func (c *client) setPrompt(prompt string) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.readLine.SetPrompt(prompt)
	c.readLine.Refresh()
}

func (c *client) setChatPrompt() {
	c.setPrompt(c.userName + " > ")
}

func (c *client) print(s string) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.readLine.Clean()
	c.readLine.Terminal.Print(s + "\n")
	c.readLine.Refresh()
}

func (c *client) readOneLine() (string, error) {
	s, err := c.readLine.Readline()
	if err == nil {
		c.mtx.Lock()
		defer c.mtx.Unlock()
		c.readLine.Terminal.Print("\033[1A\033[0J")
	}
	return s, err
}

func (c *client) exit(err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.readLine.Clean()
	log.Fatalf("退出: %v", err)
}

func (c *client) bell() {
	c.readLine.Terminal.Bell()
}

func (c *client) loop() error {
	for {
		switch c.state {
		case stateLogin:
			c.setPrompt("请输入用户名 > ")

			userName, err := c.readOneLine()
			if err != nil {
				if err == readline.ErrInterrupt {
					return ErrTerminated
				}
				return errors.WithMessage(err, "无法读取用户名")
			}

			if len(userName) == 0 {
				continue
			}

			session, err := gnet.ConnectTCP("tcp", *serverAddr)
			if err != nil {
				return errors.WithMessage(err, "无法连接服务器")
			}

			c.client = client2.NewClient(c)
			if err := c.client.Start(session, userName, opt); err != nil {
				c.print(fmt.Sprintf("登陆服务器失败: %v", err))
				continue
			}

			c.print("登陆成功，可以开始聊天了！")
			c.userName = userName
			c.state = stateChat
		case stateChat:
			c.setChatPrompt()

			msg, err := c.readOneLine()
			if err != nil {
				if err == readline.ErrInterrupt {
					return ErrTerminated
				}
				c.print(fmt.Sprintf("无法读取内容: %v", err))
				continue
			}

			if len(msg) == 0 {
				continue
			}
			if len(msg) > chat.MaxLengthOfChatContent {
				c.print(fmt.Sprintf("内容过长，请重新输入！"))
				continue
			}

			chatProtoc := &protocol.SendMessageReq{Message: msg}
			req, err := c.client.SendRequest(chatProtoc)
			if err != nil {
				c.print(fmt.Sprintf("发送聊天请求失败: %v", err))
				continue
			}

			rsp, err := req.WaitResponse()
			if err != nil {
				c.print(fmt.Sprintf("发送聊天请求失败: %v", err))
				continue
			}

			rspProtoc, ok := rsp.(*protocol.SendMessageRsp)
			if !ok {
				c.print("发送聊天请求失败: response type wrong")
				continue
			}

			if rspProtoc.Error != "" {
				c.print("发送聊天请求失败: " + rspProtoc.Error)
			}
		}
	}
}

func (c *client) OnNotify(p protocol.Protocol) {
	switch protoc := p.(type) {
	case *protocol.MessageNotify:
		t := time.UnixMilli(protoc.TimeMS)
		switch protoc.Type {
		case protocol.MsgTypeUserEnter:
			c.print(fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d.%03d \"%s\"加入聊天室！",
				t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), protoc.TimeMS%1000, protoc.UserName))
		case protocol.MsgTypeUserExit:
			c.print(fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d.%03d \"%s\"离开聊天室！",
				t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), protoc.TimeMS%1000, protoc.UserName))
		case protocol.MsgTypeUserMessage:
			c.print(fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d.%03d %s: %s",
				t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), protoc.TimeMS%1000, protoc.UserName, protoc.Message))
		}
		c.bell()
	}
}

func (c *client) OnClose(err error) {
	c.exit(err)
}
