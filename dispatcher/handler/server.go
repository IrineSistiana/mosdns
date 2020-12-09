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
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/miekg/dns"
	"time"
)

type ServerHandler interface {
	ServeDNS(ctx context.Context, qCtx *Context, w ResponseWriter)
}

type ResponseWriter interface {
	Write(m *dns.Msg) (n int, err error)
}

// DefaultServerHandler
// If entry returns an err, a SERVFAIL response will be sent back to client.
type DefaultServerHandler struct {
	Entry string
}

func (h *DefaultServerHandler) ServeDNS(ctx context.Context, qCtx *Context, w ResponseWriter) {
	queryStart := time.Now()
	err := Walk(ctx, qCtx, h.Entry)
	rtt := time.Since(queryStart).Milliseconds()
	mlog.Entry().Debugf("%v: entry %s returned after %dms:", qCtx, h.Entry, rtt)

	var r *dns.Msg
	if err != nil {
		mlog.Entry().Warnf("%v: query failed with %v", qCtx, err)
		r = new(dns.Msg)
		r.SetReply(qCtx.Q)
		r.Rcode = dns.RcodeServerFailure
	} else {
		r = qCtx.R
	}

	if r != nil {
		if _, err := w.Write(r); err != nil {
			mlog.Entry().Warnf("%v: failed to respond client: %v", qCtx, err)
		}
	}
}
