package rpc

import (
	"errors"
)

// ErrMethodArgsType 表示方法参数类型错误.
var ErrMethodArgsType = errors.New("rpc: method args type wrong")

// ErrMethodReplyType 表示方法返回值类型错误.
var ErrMethodReplyType = errors.New("rpc: method reply type wrong")

// Method 封装RPC方法需要实现的功能.
type Method interface {
	// MethodId 方法ID.
	MethodId() uint64

	// OnCall 方法被调用时的具体逻辑.
	OnCall(args any) (reply any, err error)
}

// MethodG RPC方法的泛型封装.
type MethodG[Args, Reply any] struct {
	methodId uint16                      // method id.
	fn       func(*Args) (*Reply, error) // 方法的具体逻辑.
}

// NewMethodG 构造泛型RPC方法.
func NewMethodG[Args, Reply any](methodId uint16, fn func(*Args) (*Reply, error)) *MethodG[Args, Reply] {
	if fn == nil {
		panic("rpc.NewMethodG: fn nil")
	}
	return &MethodG[Args, Reply]{
		methodId: methodId,
		fn:       fn,
	}
}

// MethodId 方法ID.
func (m *MethodG[Args, Reply]) MethodId() uint16 {
	return m.methodId
}

// OnCall 方法被调用逻辑.
func (m *MethodG[Args, Reply]) OnCall(args any) (reply any, err error) {
	a, ok := args.(*Args)
	if !ok {
		return nil, ErrMethodArgsType
	}
	return m.fn(a)
}

// Call 同步调用方法.
func (m *MethodG[Args, Reply]) Call(c *Client, args *Args, timeout int64) (reply *Reply, err error) {
	if r, err := c.Call(m.methodId, args, timeout); err != nil {
		return nil, err
	} else if rr, ok := r.(*Reply); !ok {
		return nil, ErrMethodReplyType
	} else {
		return rr, nil
	}
}

// AsyncCall 异步调用方法.
func (m *MethodG[Args, Reply]) AsyncCall(c *Client, args *Args, timeout int64, callback func(reply *Reply, err error)) error {
	return c.AsyncCall(m.methodId, args, timeout, func(reply any, err error) {
		if err != nil {
			callback(nil, err)
			return
		} else if rr, ok := reply.(*Reply); !ok {
			callback(nil, ErrMethodReplyType)
			return
		} else {
			callback(rr, nil)
		}
	})
}
