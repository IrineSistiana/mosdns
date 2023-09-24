/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package query_context

import (
	"sync/atomic"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/server"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Context is a query context that pass through plugins
// A Context will always have a non-nil Q.
// Context MUST be created using NewContext.
// All Context funcs are not safe for concurrent use.
type Context struct {
	// id for this Context. Not for the dns query. This id is mainly for logging.
	id        uint32
	startTime time.Time // when was this Context created
	q         *dns.Msg

	// EDNS0 options from client.
	// Note: Q() is the query that is going to forward. Initially it has no
	// option. All options from client are moved to QueryOpt.
	// Plugins that responsible for handling edns0 option should 
	// check QueryOpt and add options into Q() on demand.
	// This field should be read-only.
	QueryOpt []dns.EDNS0
	// Server Info. This field should be read-only.
	ServerMeta ServerMeta

	// Response. Might be nil.
	r *dns.Msg

	// lazy init.
	kv    map[uint32]any
	marks map[uint32]struct{}
}

var contextUid atomic.Uint32

type ServerMeta = server.QueryMeta

// NewContext creates a new query Context.
// q is the query dns msg. It cannot be nil, or NewContext will panic.
func NewContext(q *dns.Msg) *Context {
	if q == nil {
		panic("handler: query msg is nil")
	}
	ctx := &Context{
		q:         q,
		id:        contextUid.Add(1),
		startTime: time.Now(),
	}

	return ctx
}

// Q returns the query msg. It always returns a non-nil msg.
func (ctx *Context) Q() *dns.Msg {
	return ctx.q
}

// R returns the response. It might be nil.
func (ctx *Context) R() *dns.Msg {
	return ctx.r
}

// SetResponse stores the response r to the context.
// Note: It just stores the pointer of r. So the caller
// MUST NOT modify or read r after the call.
func (ctx *Context) SetResponse(r *dns.Msg) {
	ctx.r = r
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

// InfoField returns a zap.Field contains a brief summary of this Context.
// Useful in log.
func (ctx *Context) InfoField() zap.Field {
	return zap.Object("query", ctx)
}

// Copy deep copies this Context.
// See CopyTo.
func (ctx *Context) Copy() *Context {
	newCtx := new(Context)
	ctx.CopyTo(newCtx)
	return newCtx
}

// CopyTo deep copies this Context to d.
// Note that values that stored by StoreValue is not deep-copied.
func (ctx *Context) CopyTo(d *Context) *Context {
	d.startTime = ctx.startTime
	d.q = ctx.q.Copy()
	d.id = ctx.id

	if r := ctx.r; r != nil {
		d.r = r.Copy()
	}
	d.kv = copyMap(ctx.kv)
	d.marks = copyMap(ctx.marks)
	return d
}

// StoreValue stores any v in to this Context
// k MUST from RegKey.
func (ctx *Context) StoreValue(k uint32, v any) {
	if ctx.kv == nil {
		ctx.kv = make(map[uint32]any)
	}
	ctx.kv[k] = v
}

// GetValue returns the value stored by StoreValue.
func (ctx *Context) GetValue(k uint32) (any, bool) {
	v, ok := ctx.kv[k]
	return v, ok
}

// DeleteValue deletes value k from Context
func (ctx *Context) DeleteValue(k uint32) {
	delete(ctx.kv, k)
}

// SetMark marks this Context with given mark.
func (ctx *Context) SetMark(m uint32) {
	if ctx.marks == nil {
		ctx.marks = make(map[uint32]struct{})
	}
	ctx.marks[m] = struct{}{}
}

// HasMark reports whether this mark m was marked by SetMark.
func (ctx *Context) HasMark(m uint32) bool {
	_, ok := ctx.marks[m]
	return ok
}

// DeleteMark deletes mark m from this Context.
func (ctx *Context) DeleteMark(m uint32) {
	delete(ctx.marks, m)
}

// MarshalLogObject implements zapcore.ObjectMarshaler.
func (ctx *Context) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddUint32("uqid", ctx.id)

	if clientAddr := ctx.ServerMeta.ClientAddr; clientAddr.IsValid() {
		zap.Stringer("client", clientAddr).AddTo(encoder)
	}

	q := ctx.Q()
	if len(q.Question) != 1 {
		encoder.AddBool("odd_question", true)
	} else {
		question := q.Question[0]
		encoder.AddString("qname", question.Name)
		encoder.AddUint16("qtype", question.Qtype)
		encoder.AddUint16("qclass", question.Qclass)
	}
	if r := ctx.R(); r != nil {
		encoder.AddInt("rcode", r.Rcode)
	}
	encoder.AddDuration("elapsed", time.Since(ctx.StartTime()))
	return nil
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	cm := make(map[K]V, len(m))
	for k, v := range m {
		cm[k] = v
	}
	return cm
}
