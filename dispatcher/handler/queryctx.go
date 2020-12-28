//     Copyright (C) 2020, IrineSistiana
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
	"github.com/miekg/dns"
	"net"
	"strings"
	"time"
)

// Context is a query context that pass through plugins
// A Context will always have a non-nil Q.
// Context MUST be created by NewContext.
type Context struct {
	Q    *dns.Msg
	From net.Addr

	Status ContextStatus
	R      *dns.Msg

	startTime time.Time
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
	ContextStatusWaitingResponse: "WaitingResponse",
	ContextStatusResponded:       "Responded",
	ContextStatusServerFailed:    "ServerFailed",
	ContextStatusDropped:         "Dropped",
	ContextStatusRejected:        "Rejected",
}

func (status ContextStatus) String() string {
	s, ok := statusToStr[status]
	if ok {
		return s
	}
	return fmt.Sprintf("invalid status %d", status)
}

func NewContext(q *dns.Msg) *Context {
	return &Context{Q: q, Status: ContextStatusWaitingResponse, startTime: time.Now()}
}

func (ctx *Context) SetResponse(r *dns.Msg, status ContextStatus) {
	ctx.R = r
	ctx.Status = status
}

func (ctx *Context) Copy() *Context {
	if ctx == nil {
		return nil
	}

	newCtx := new(Context)

	if ctx.Q != nil {
		newCtx.Q = ctx.Q.Copy()
	}
	newCtx.From = ctx.From

	newCtx.Status = ctx.Status
	if ctx.R != nil {
		newCtx.R = ctx.R.Copy()
	}
	newCtx.startTime = ctx.startTime

	return newCtx
}

func (ctx *Context) String() string {
	if ctx == nil {
		return "<nil>"
	}
	sb := new(strings.Builder)
	sb.Grow(128)

	sb.WriteString(fmt.Sprintf("%v, id: %d, t: %d ms", ctx.Q.Question, ctx.Q.Id, time.Since(ctx.startTime).Milliseconds()))
	if ctx.From != nil {
		sb.WriteString(fmt.Sprintf(", from: %s://%s", ctx.From.Network(), ctx.From.String()))
	}
	return sb.String()
}
