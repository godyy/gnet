package client

import (
	"container/heap"
	"time"

	"github.com/godyy/gnet/internal/examples/chat/protocol"
)

type Request struct {
	msgSeri uint32
	timeout time.Time
	Err     error
	Rsp     protocol.Protocol
	Done    chan struct{}
	heapIdx int
}

func newRequest(msgSeri uint32, timeout time.Time) *Request {
	return &Request{
		msgSeri: msgSeri,
		timeout: timeout,
		Done:    make(chan struct{}),
	}
}

func (r *Request) response(rsp protocol.Protocol) {
	r.Rsp = rsp
	close(r.Done)
}

func (r *Request) error(err error) {
	r.Err = err
	close(r.Done)
}

func (r *Request) WaitResponse() (protocol.Protocol, error) {
	_, _ = <-r.Done
	return r.Rsp, r.Err
}

type requestHeapSlice []*Request

func (h requestHeapSlice) Len() int {
	return len(h)
}

func (h requestHeapSlice) Less(i, j int) bool {
	return h[i].timeout.Before(h[j].timeout)
}

func (h requestHeapSlice) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIdx = i
	h[j].heapIdx = j
}

func (h *requestHeapSlice) Push(x any) {
	xx := x.(*Request)
	*h = append(*h, xx)
	xx.heapIdx = h.Len() - 1
}

func (h *requestHeapSlice) Pop() any {
	n := h.Len() - 1
	x := (*h)[n]
	*h = (*h)[:n]
	return x
}

type requestHeap struct {
	s requestHeapSlice
}

func newRequestHeap(cap int) *requestHeap {
	return &requestHeap{s: make([]*Request, 0, cap)}
}

func (h *requestHeap) len() int {
	return h.s.Len()
}

func (h *requestHeap) push(x *Request) {
	heap.Push(&h.s, x)
}

func (h *requestHeap) pop() *Request {
	return heap.Pop(&h.s).(*Request)
}

func (h *requestHeap) top() *Request {
	return h.s[0]
}

func (h *requestHeap) remove(i int) {
	heap.Remove(&h.s, i)
}
