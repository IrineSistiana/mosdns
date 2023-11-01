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

package dual_selector

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type dummyNext struct {
	returnA     bool
	latencyA    time.Duration
	returnAAAA  bool
	latencyAAAA time.Duration
}

func (d *dummyNext) Exec(_ context.Context, qCtx *query_context.Context) error {
	q := qCtx.Q()
	r := new(dns.Msg)
	r.SetReply(q)
	question := q.Question[0]
	rrh := dns.RR_Header{
		Name:   question.Name,
		Rrtype: question.Qtype,
		Class:  question.Qclass,
	}

	if question.Qtype == dns.TypeA && d.returnA {
		r.Answer = append(r.Answer, &dns.A{
			Hdr: rrh,
			A:   net.IPv4(1, 2, 3, 4),
		})
		time.Sleep(d.latencyA)
	}
	if question.Qtype == dns.TypeAAAA && d.returnAAAA {
		r.Answer = append(r.Answer, &dns.AAAA{
			Hdr:  rrh,
			AAAA: net.IPv4(1, 2, 3, 4),
		})
		time.Sleep(d.latencyAAAA)
	}
	qCtx.SetResponse(r)
	return nil
}

func TestSelector_Exec(t *testing.T) {
	nextNoA := &dummyNext{
		returnA:    false,
		returnAAAA: true,
	}
	nextNoAAAA := &dummyNext{
		returnA:    true,
		returnAAAA: false,
	}
	nextDual := &dummyNext{
		returnA:    true,
		returnAAAA: true,
	}
	nextLateA := &dummyNext{
		returnA:    true,
		latencyA:   time.Millisecond * 1000,
		returnAAAA: true,
	}
	nextLateAAAA := &dummyNext{
		returnA:     true,
		returnAAAA:  true,
		latencyAAAA: time.Millisecond * 1000,
	}

	tests := []struct {
		name      string
		prefer    uint16
		qtype     uint16
		next      *dummyNext
		wantErr   bool
		wantReply bool
	}{
		{
			name:      "prefer v4: do not block domain AAAA if domain does not have an A record",
			prefer:    dns.TypeA,
			qtype:     dns.TypeAAAA,
			next:      nextNoA,
			wantErr:   false,
			wantReply: true,
		},
		{
			name:      "prefer v4: do not block domain AAAA if A reply wasn't returned on time",
			prefer:    dns.TypeA,
			qtype:     dns.TypeAAAA,
			next:      nextLateA,
			wantErr:   false,
			wantReply: true,
		},
		{
			name:      "prefer v4: block domain AAAA if domain has A records",
			prefer:    dns.TypeA,
			qtype:     dns.TypeAAAA,
			next:      nextDual,
			wantErr:   false,
			wantReply: false,
		},
		{
			name:      "prefer v6: do not block domain A if domain does not have an AAAA record",
			prefer:    dns.TypeAAAA,
			qtype:     dns.TypeA,
			next:      nextNoAAAA,
			wantErr:   false,
			wantReply: true,
		},
		{
			name:      "prefer v6: do not block domain A if AAAA reply wasn't returned on time",
			prefer:    dns.TypeAAAA,
			qtype:     dns.TypeA,
			next:      nextLateAAAA,
			wantErr:   false,
			wantReply: true,
		},
		{
			name:      "prefer v6: block domain A if domain has AAAA records",
			prefer:    dns.TypeAAAA,
			qtype:     dns.TypeA,
			next:      nextDual,
			wantErr:   false,
			wantReply: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSelector(sequence.NewBQ(coremain.NewTestMosdnsWithPlugins(nil), zap.NewNop()), tt.prefer)

			q := new(dns.Msg)
			q.SetQuestion("example.", tt.qtype)
			qCtx := query_context.NewContext(q)
			cw := sequence.NewChainWalker([]*sequence.ChainNode{{E: tt.next}}, nil)
			if err := s.Exec(context.Background(), qCtx, cw); (err != nil) != tt.wantErr {
				t.Errorf("Exec() error = %v, wantErr %v", err, tt.wantErr)
			}

			r := qCtx.R()
			if hasReply := msgAnsHasRR(r, tt.qtype); hasReply != tt.wantReply {
				t.Errorf("Exec() hasReply = %v, wantReply %v", hasReply, tt.wantReply)
			}
		})
	}
}
