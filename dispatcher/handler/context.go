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
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/dnsutils"
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
	startTime  time.Time // when this Context was created
	q          *dns.Msg
	id         uint32 // additional uint32 to distinguish duplicated msg
	clientAddr net.Addr

	status ContextStatus
	r      *dns.Msg
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

// String returns a short summery of its query.
func (ctx *Context) String() string {
	q := ctx.q
	if len(q.Question) == 1 {
		q := q.Question[0]
		return fmt.Sprintf("%s %s %s %d %d %s", q.Name, dnsutils.QclassToString(q.Qclass), dnsutils.QtypeToString(q.Qtype), ctx.q.Id, ctx.id, ctx.clientAddr)
	}
	return fmt.Sprintf("%v %d %d %s", ctx.q.Question, ctx.q.Id, ctx.id, ctx.clientAddr)
}

// Q returns the query msg. It always returns a non-nil msg.
func (ctx *Context) Q() *dns.Msg {
	return ctx.q
}

// From returns the client net.Addr. It might be nil.
func (ctx *Context) From() net.Addr {
	return ctx.clientAddr
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
func (ctx *Context) Copy() *Context {
	newCtx := new(Context)
	ctx.CopyTo(newCtx)
	return newCtx
}

// CopyTo deep copies this Context to d.
func (ctx *Context) CopyTo(d *Context) *Context {
	d.startTime = ctx.startTime
	d.q = ctx.q.Copy()
	d.id = ctx.id
	d.clientAddr = ctx.clientAddr
	d.status = ctx.status
	if r := ctx.r; r != nil {
		d.r = r.Copy()
	}
	return d
}
