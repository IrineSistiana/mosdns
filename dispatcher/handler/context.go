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
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

// RequestMeta represents some metadata about the request.
type RequestMeta struct {
	// ClientIP contains the client ip address.
	ClientIP net.IP

	// FromUDP indicates the request is from an udp socket.
	FromUDP bool
}

// Context is a query context that pass through plugins
// A Context will always have a non-nil Q.
// Context MUST be created by NewContext.
type Context struct {
	// init at beginning
	startTime     time.Time // when this Context was created
	q             *dns.Msg
	originalQuery *dns.Msg
	id            uint32 // additional uint32 to distinguish duplicated msg
	reqMeta       *RequestMeta

	status ContextStatus
	r      *dns.Msg
	marks  map[uint]struct{}
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
var zeroMeta = &RequestMeta{}

// NewContext creates a new query Context.
// q is the query dns msg. It cannot be nil, or NewContext will panic.
// meta can be nil.
func NewContext(q *dns.Msg, meta *RequestMeta) *Context {
	if q == nil {
		panic("handler: query msg is nil")
	}

	if meta == nil {
		meta = zeroMeta
	}

	ctx := &Context{
		q:             q,
		originalQuery: q.Copy(),
		reqMeta:       meta,
		id:            atomic.AddUint32(&id, 1),
		startTime:     time.Now(),

		status: ContextStatusWaitingResponse,
	}

	return ctx
}

// String returns a short summery of its query.
func (ctx *Context) String() string {
	var question string
	var clientAddr string

	if len(ctx.q.Question) >= 1 {
		q := ctx.q.Question[0]
		question = fmt.Sprintf("%s %s %s", q.Name, dnsutils.QclassToString(q.Qclass), dnsutils.QtypeToString(q.Qtype))
	} else {
		question = "empty question"
	}
	if ctx.reqMeta.ClientIP != nil {
		clientAddr = ctx.reqMeta.ClientIP.String()
	} else {
		clientAddr = "unknown client"
	}

	return fmt.Sprintf("%s %d %d %s", question, ctx.q.Id, ctx.id, clientAddr)
}

// Q returns the query msg. It always returns a non-nil msg.
func (ctx *Context) Q() *dns.Msg {
	return ctx.q
}

// OriginalQuery returns the copied original query msg a that created the Context.
// It always returns a non-nil msg.
// The returned msg SHOULD NOT be modified.
func (ctx *Context) OriginalQuery() *dns.Msg {
	return ctx.q
}

// ReqMeta returns the request metadata.
// The returned *RequestMeta is a reference shared by all ReqMeta.
// Caller must not modify it.
func (ctx *Context) ReqMeta() *RequestMeta {
	return ctx.reqMeta
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
	d.originalQuery = ctx.originalQuery
	d.reqMeta = ctx.reqMeta
	d.id = ctx.id

	d.status = ctx.status
	if r := ctx.r; r != nil {
		d.r = r.Copy()
	}
	return d
}

// AddMark adds mark m to this Context.
func (ctx *Context) AddMark(m uint) {
	if ctx.marks == nil {
		ctx.marks = make(map[uint]struct{})
	}
	ctx.marks[m] = struct{}{}
}

// HasMark reports whether this Context has mark m.
func (ctx *Context) HasMark(m uint) bool {
	_, ok := ctx.marks[m]
	return ok
}
