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
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"time"
)

type ServerHandler interface {
	// ServeDNS use ctx to control deadline, exchange qCtx, and write response to w.
	ServeDNS(ctx context.Context, qCtx *Context, w ResponseWriter)
}

// ResponseWriter can write msg to the client.
type ResponseWriter interface {
	Write(m *dns.Msg) (n int, err error)
}

// DefaultServerHandler
// If entry returns an err, a SERVFAIL response will be sent back to client.
type DefaultServerHandler struct {
	Entry  string
	Logger *logrus.Entry
}

// ServeDNS: see DefaultServerHandler.
func (h *DefaultServerHandler) ServeDNS(ctx context.Context, qCtx *Context, w ResponseWriter) {
	start := time.Now()
	err := Walk(ctx, qCtx, h.Entry)
	if err != nil {
		h.Logger.Warnf("entry %s returned after %d ms with err: %v", h.Entry, time.Since(start).Milliseconds(), err)
	} else {
		h.Logger.Debugf("entry %s returned after %d ms", h.Entry, time.Since(start).Milliseconds())
	}

	var r *dns.Msg
	if err != nil {
		r = new(dns.Msg)
		r.SetReply(qCtx.Q)
		r.Rcode = dns.RcodeServerFailure
	} else {
		r = qCtx.R
	}

	if r != nil {
		if _, err = w.Write(r); err != nil {
			h.Logger.Warnf("response might not send back to client: %v", err)
		}
	}
}
