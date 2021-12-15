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

package padding

import (
	"context"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
)

const PluginType = "padding"

func init() {
	handler.MustRegPlugin(&PadQuery{BP: handler.NewBP("_pad_query", PluginType)}, true)
	handler.MustRegPlugin(&ResponsePaddingHandler{BP: handler.NewBP("_enable_conditional_response_padding", PluginType)}, true)
	handler.MustRegPlugin(&ResponsePaddingHandler{BP: handler.NewBP("_enable_response_padding", PluginType), Always: true}, true)
}

var _ handler.ExecutablePlugin = (*PadQuery)(nil)
var _ handler.ExecutablePlugin = (*ResponsePaddingHandler)(nil)

const (
	minimumQueryLen    = 128
	minimumResponseLen = 468
)

type PadQuery struct {
	*handler.BP
}

// Exec pads queries to 128 octets as RFC 8467 recommended.
func (p *PadQuery) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	q := qCtx.Q()
	dnsutils.PadToMinimum(q, minimumQueryLen)

	if err := handler.ExecChainNode(ctx, qCtx, next); err != nil {
		return err
	}
	if r := qCtx.R(); r != nil {
		oq := qCtx.OriginalQuery()
		opt := oq.IsEdns0()
		if opt == nil { // The original query does not have EDNS0
			dnsutils.RemoveEDNS0(r) // Remove EDNS0 from the response.
		} else {
			if dnsutils.GetEDNS0Option(opt, dns.EDNS0PADDING) == nil { // The original query does not have Padding option.
				if opt := r.IsEdns0(); opt != nil { // Remove Padding from the response.
					dnsutils.RemoveEDNS0Option(opt, dns.EDNS0PADDING)
				}
			}
		}
	}
	return nil
}

type ResponsePaddingHandler struct {
	*handler.BP
	// Always indicates that ResponsePaddingHandler should always
	// pad response as long as it is EDNS0 even if it wasn't padded.
	Always bool
}

// Exec pads responses to 468 octets as RFC 8467 recommended.
func (h *ResponsePaddingHandler) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	if err := handler.ExecChainNode(ctx, qCtx, next); err != nil {
		return err
	}

	oq := qCtx.OriginalQuery()
	if r := qCtx.R(); r != nil {
		opt := oq.IsEdns0()
		if opt != nil { // Only pad response if client supports EDNS0.
			if h.Always {
				dnsutils.PadToMinimum(r, minimumResponseLen)
			} else {
				// Only pad response if client padded its query.
				if dnsutils.GetEDNS0Option(opt, dns.EDNS0PADDING) != nil {
					dnsutils.PadToMinimum(r, minimumResponseLen)
				}
			}
		}
	}
	return nil
}
