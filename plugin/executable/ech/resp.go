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

package ech

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"slices"
	"time"
)

func emptyResponse(qCtx *query_context.Context) *dns.Msg {
	r := &dns.Msg{}
	r.SetReply(qCtx.Q())
	qCtx.SetResponse(r)
	return r
}

func (p *ECH) httpsRR(rr *rrLocal, qname string) *dns.HTTPS {
	ht := *rr.rr
	ht.Hdr.Name = qname
	if rr.ts == 0 {
		ht.Hdr.Ttl = 3600
	} else {
		ht.Hdr.Ttl = uint32(max(int64(rr.ttl)+rr.ts-time.Now().Unix(), 0))
	}
	return &ht
}

func (p *ECH) directResponse(rr *rrLocal, qname string, qCtx *query_context.Context) error {
	r := emptyResponse(qCtx)
	ht := p.httpsRR(rr, qname)
	r.Answer = append(r.Answer, ht)

	p.l.Debug("ech direct response",
		zap.String("forward_tag", p.impl.tags.forward),
		zap.String("ech_qname", p.impl.qname),
		zap.String("qname", qname))
	return nil
}

func (p *ECH) filterResponse(rr *rrLocal, r *dns.Msg, qname string) error {
	if r.Rcode != dns.RcodeSuccess {
		p.l.Debug("ech upstream failure, skipping",
			zap.String("forward_tag", p.impl.tags.forward),
			zap.String("ech_qname", p.impl.qname),
			zap.String("qname", qname),
			zap.String("rcode", dns.RcodeToString[r.Rcode]))
		return nil
	}

	p.l.Debug("ech filtering response",
		zap.String("forward_tag", p.impl.tags.forward),
		zap.String("ech_qname", p.impl.qname),
		zap.String("qname", qname))

	var fh bool
	for _, a := range r.Answer {
		if a.Header().Rrtype == dns.TypeHTTPS {
			fh = true
			if err := p.addECHIfNotPresent(a.(*dns.HTTPS), rr.ech); err != nil {
				return err
			}
			continue
		}
	}
	if !fh {
		p.l.Debug("ech filtering response, no HTTPS record found, adding",
			zap.String("forward_tag", p.impl.tags.forward),
			zap.String("ech_qname", p.impl.qname),
			zap.String("qname", qname))
		ht := p.httpsRR(rr, qname)
		if err := p.addECHIfNotPresent(ht, rr.ech); err != nil {
			return err
		}
		r.Answer = append(r.Answer, ht)
	}

	return nil
}

func (p *ECH) addECHIfNotPresent(ht *dns.HTTPS, ech *dns.SVCBECHConfig) error {
	found := slices.ContainsFunc(ht.Value, func(kv dns.SVCBKeyValue) bool { return kv.Key() == dns.SVCB_ECHCONFIG })
	if !found {
		p.l.Debug("ech filter response, no ECHConfig, adding",
			zap.String("forward_tag", p.impl.tags.forward),
			zap.String("ech_qname", p.impl.qname))
		if ht == nil {
			p.l.Warn("ech failed: empty ECHConfigList, configuration or network error",
				zap.String("forward_tag", p.impl.tags.forward),
				zap.String("ech_qname", p.impl.qname))
			return fmt.Errorf("ech failed: empty ECHConfigList, configuration or network error")
		} else {
			ht.Value = append(ht.Value, ech)
		}
	}

	return nil
}
