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

const (
	edns0Size = 1200
)

// Context is a query context that pass through plugins.
// All Context funcs are not safe for concurrent use.
type Context struct {
	id        uint32
	startTime time.Time

	// ServerMeta contains some meta info from the server.
	// It is read-only.
	ServerMeta ServerMeta
	query      *dns.Msg // always has one question.
	clientOpt  *dns.OPT // may be nil

	resp        *dns.Msg
	respOpt     *dns.OPT // nil if clientOpt == nil
	upstreamOpt *dns.OPT // may be nil

	// lazy init.
	kv    map[uint32]any
	marks map[uint32]struct{}
}

var contextUid atomic.Uint32

type ServerMeta = server.QueryMeta

// NewContext creates a new query Context.
// q must have one question.
// NewContext takes the ownership of q.
func NewContext(q *dns.Msg) *Context {
	ctx := &Context{
		id:        contextUid.Add(1),
		startTime: time.Now(),
		query:     q,
		clientOpt: addNewAndSwapOldOpt(q),
	}
	if ctx.clientOpt != nil {
		ctx.respOpt = newOpt()

		// RFC 3225 3
		// The DO bit of the query MUST be copied in the response.
		if ctx.clientOpt.Do() {
			setDo(ctx.respOpt, true)
		}
	}
	return ctx
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

// Q returns the query msg that will be forward to upstream.
// It always returns a non-nil msg with one question and EDNS0 OPT.
// If Caller want to modify the msg, be sure not to break those conditions.
func (ctx *Context) Q() *dns.Msg {
	return ctx.query
}

// QQuestion returns the query question.
func (ctx *Context) QQuestion() dns.Question {
	return ctx.query.Question[0]
}

// QOpt returns the query opt. It always returns a non-nil opt.
// It's a helper func for searching opt in Q() manually.
func (ctx *Context) QOpt() *dns.OPT {
	opt := findOpt(ctx.query)
	ctx.query.IsEdns0()
	if opt == nil {
		panic("query opt is missing")
	}
	return opt
}

// ClientOpt returns the OPT rr from client. Maybe nil, if client does not send it.
// Plugins that responsible for handling EDNS0 option should
// check ClientOpt and pick/add options into Q() on demand.
// The OPT is read-only.
func (ctx *Context) ClientOpt() *dns.OPT {
	return ctx.clientOpt
}

// SetResponse sets m as response. It takes the ownership of m.
// If m is nil. It removes existing response.
func (ctx *Context) SetResponse(m *dns.Msg) {
	ctx.resp = m
	if m == nil {
		ctx.upstreamOpt = nil
	} else {
		ctx.upstreamOpt = popOpt(m)
	}
}

// R returns the response that will be sent to client. It might be nil.
// Note: R does not have EDNS0. Caller MUST NOT add a dns.OPT into R.
// Use RespOpt() instead.
func (ctx *Context) R() *dns.Msg {
	return ctx.resp
}

// RespOpt returns the OPT that will be sent to client.
// If client support EDNS0, then RespOpt always returns a non-nil OPT.
// No matter what R() returns.
// Otherwise, RespOpt returns nil.
func (ctx *Context) RespOpt() *dns.OPT {
	return ctx.respOpt
}

// UpstreamOpt returns the OPT from upstream. May be nil.
// Plugins that responsible for handling EDNS0 option should
// check UpstreamOpt and pick/add options into RespOpt on demand.
// The OPT is read-only.
func (ctx *Context) UpstreamOpt() *dns.OPT {
	return ctx.upstreamOpt
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
	d.id = ctx.id
	d.startTime = ctx.startTime

	d.ServerMeta = ctx.ServerMeta
	d.query = ctx.query.Copy()
	d.clientOpt = ctx.clientOpt

	if ctx.resp != nil {
		d.resp = ctx.resp.Copy()
	}
	if ctx.respOpt != nil {
		d.respOpt = dns.Copy(ctx.respOpt).(*dns.OPT)
	}
	d.upstreamOpt = ctx.upstreamOpt

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

	question := ctx.query.Question[0]
	encoder.AddString("qname", question.Name)
	encoder.AddUint16("qtype", question.Qtype)
	encoder.AddUint16("qclass", question.Qclass)

	if r := ctx.resp; r != nil {
		encoder.AddInt("rcode", r.Rcode)
	}
	encoder.AddDuration("elapsed", time.Since(ctx.startTime))
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

func addNewAndSwapOldOpt(m *dns.Msg) *dns.OPT {
	for i := len(m.Extra) - 1; i >= 0; i-- {
		// If m has oldOpt
		if oldOpt, ok := m.Extra[i].(*dns.OPT); ok {
			// replace it directly
			m.Extra[i] = newOpt()
			return oldOpt
		}
	}
	m.Extra = append(m.Extra, newOpt())
	return nil
}

func popOpt(m *dns.Msg) *dns.OPT {
	for i := len(m.Extra) - 1; i >= 0; i-- {
		if opt, ok := m.Extra[i].(*dns.OPT); ok {
			m.Extra = append(m.Extra[:i], m.Extra[i+1:]...)
			return opt
		}
	}
	return nil
}

func findOpt(m *dns.Msg) *dns.OPT {
	for i := len(m.Extra) - 1; i >= 0; i-- {
		if opt, ok := m.Extra[i].(*dns.OPT); ok {
			return opt
		}
	}
	return nil
}

func newOpt() *dns.OPT {
	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	opt.SetUDPSize(edns0Size)
	return opt
}

func setDo(opt *dns.OPT, do bool) {
	const doBit = 1 << 15 // DNSSEC OK
	if do {
		opt.Hdr.Ttl |= doBit
	}
}
