//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package handler

import (
	"context"
	"fmt"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

// Context is a query context that pass through plugins
// A Context will always have a non-nil Q.
// Context MUST be created by NewContext.
type Context struct {
	// init at beginning
	q          *dns.Msg
	clientAddr net.Addr
	id         uint32 // additional uint to distinguish duplicated msg
	startTime  time.Time

	// tcpClient indicates that client is using a tcp-like protocol (tcp, dot etc...).
	// It means the response can have an arbitrary length and will not be truncated.
	tcpClient bool

	status ContextStatus
	r      *dns.Msg

	deferrable  []Executable
	deferAtomic uint32
}

type ContextStatus uint8

const (
	ContextStatusWaitingResponse ContextStatus = iota
	ContextStatusResponded
	ContextStatusServerFailed
	ContextStatusDropped
	ContextStatusRejected
)

var statusToStr = map[ContextStatus]string{
	ContextStatusWaitingResponse: "waiting response",
	ContextStatusResponded:       "responded",
	ContextStatusServerFailed:    "server failed",
	ContextStatusDropped:         "dropped",
	ContextStatusRejected:        "rejected",
}

func (status ContextStatus) String() string {
	s, ok := statusToStr[status]
	if ok {
		return s
	}
	return strconv.Itoa(int(status))
}

var id uint32

// NewContext creates a new query Context.
// q is the query dns msg. It cannot be nil, or NewContext will panic.
// from is the client net.Addr. It can be nil.
func NewContext(q *dns.Msg, from net.Addr) *Context {
	if q == nil {
		panic("handler: query msg is nil")
	}

	ctx := &Context{
		q:          q,
		clientAddr: from,
		id:         atomic.AddUint32(&id, 1),
		startTime:  time.Now(),

		status: ContextStatusWaitingResponse,
	}

	return ctx
}

func (ctx *Context) String() string {
	q := ctx.q
	if len(q.Question) == 1 {
		q := q.Question[0]
		return fmt.Sprintf("%s %d %d %d %d", q.Name, q.Qtype, q.Qclass, ctx.q.Id, ctx.id)
	}
	return fmt.Sprintf("%v %d %d", ctx.q.Question, ctx.id, ctx.q.Id)
}

// Q returns the query msg. It always returns a non-nil msg.
func (ctx *Context) Q() *dns.Msg {
	return ctx.q
}

// From returns the client net.Addr. It might be nil.
func (ctx *Context) From() net.Addr {
	return ctx.clientAddr
}

func (ctx *Context) SetTCPClient(b bool) {
	ctx.tcpClient = b
}

func (ctx *Context) IsTCPClient() bool {
	return ctx.tcpClient
}

// R returns the response. It might be nil.
func (ctx *Context) R() *dns.Msg {
	return ctx.r
}

// Status returns the context status.
func (ctx *Context) Status() ContextStatus {
	return ctx.status
}

// SetResponse stores the response r to the context.
// Note: It just stores the pointer of r. So the caller
// shouldn't modify or read r after the call.
func (ctx *Context) SetResponse(r *dns.Msg, status ContextStatus) {
	ctx.r = r
	ctx.status = status
}

// CopyDeferFrom copies defer Executable from src.
func (ctx *Context) CopyDeferFrom(src *Context) {
	ctx.deferrable = make([]Executable, len(src.deferrable))
	copy(ctx.deferrable, src.deferrable)
}

// DeferExec registers an deferred Executable at this Context.
func (ctx *Context) DeferExec(e Executable) {
	if i := atomic.LoadUint32(&ctx.deferAtomic); i == 1 {
		panic("handler Context: concurrent ExecDefer or DeferExec")
	}
	ctx.deferrable = append(ctx.deferrable, &deferExecutable{e: e})
}

type deferExecutable struct {
	e Executable
}

func (d *deferExecutable) Exec(ctx context.Context, qCtx *Context) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	return d.e.Exec(ctx, qCtx)
}

// ExecDefer executes all deferred Executable registered by DeferExec.
func (ctx *Context) ExecDefer(cCtx context.Context) error {
	if len(ctx.deferrable) == 0 {
		return nil
	}

	if ok := atomic.CompareAndSwapUint32(&ctx.deferAtomic, 0, 1); !ok {
		panic("handler Context: concurrent ExecDefer or DeferExec")
	}
	defer atomic.CompareAndSwapUint32(&ctx.deferAtomic, 1, 0)

	for i := range ctx.deferrable {
		executable := ctx.deferrable[len(ctx.deferrable)-1]
		ctx.deferrable[len(ctx.deferrable)-1] = nil
		ctx.deferrable = ctx.deferrable[0 : len(ctx.deferrable)-1]
		if err := executable.Exec(cCtx, ctx); err != nil {
			return fmt.Errorf("defer exec #%d: %w", i, err)
		}
	}
	return nil
}

// Id returns the Context id.
// Note: This id is not the dns msg id.
// It's a unique uint32 growing with the number of query.
func (ctx *Context) Id() uint32 {
	return ctx.id
}

// StartTime returns the time when the Context was created.
func (ctx *Context) StartTime() time.Time {
	return ctx.startTime
}

// InfoField returns a zap.Field.
// Just for convenience.
func (ctx *Context) InfoField() zap.Field {
	return zap.Stringer("query", ctx)
}

// Copy deep copies this Context.
// Note that Copy won't copy registered deferred Executable.
// To copy them, use CopyDeferFrom after Copy.
func (ctx *Context) Copy() *Context {
	newCtx := ctx.CopyNoR()
	if ctx.r != nil {
		newCtx.r = ctx.r.Copy()
	}

	return newCtx
}

// CopyNoR deep copies this Context. Except deferred Executable
// and response.
func (ctx *Context) CopyNoR() *Context {
	newCtx := new(Context)

	newCtx.q = ctx.q.Copy()
	newCtx.clientAddr = ctx.clientAddr
	newCtx.id = ctx.id
	newCtx.startTime = ctx.startTime
	newCtx.tcpClient = ctx.tcpClient
	newCtx.status = ctx.status

	return newCtx
}

type PipeContext struct {
	logger *zap.Logger
	s      []string

	index int
}

func NewPipeContext(s []string, logger *zap.Logger) *PipeContext {
	return &PipeContext{s: s, logger: logger}
}

func (c *PipeContext) ExecNextPlugin(ctx context.Context, qCtx *Context) error {
	for c.index < len(c.s) {
		tag := c.s[c.index]
		p, err := GetPlugin(tag)
		if err != nil {
			return err
		}
		c.index++
		switch {
		case p.Is(PITContextConnector):
			return p.Connect(ctx, qCtx, c)
		case p.Is(PITESExecutable):
			earlyStop, err := p.ExecES(ctx, qCtx)
			if earlyStop || err != nil {
				return err
			}
		default:
			return fmt.Errorf("plugin %s class err", tag)
		}
	}
	return nil
}
