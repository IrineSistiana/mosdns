package dual_selector

import (
	"context"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/miekg/dns"
	"net"
	"testing"
	"time"
)

type dummyNext struct {
	returnA     bool
	latencyA    time.Duration
	returnAAAA  bool
	latencyAAAA time.Duration
}

func (d *dummyNext) Exec(_ context.Context, qCtx *handler.Context, _ handler.ExecutableChainNode) error {
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
	qCtx.SetResponse(r, handler.ContextStatusResponded)
	return nil
}

func TestSelector_Exec(t *testing.T) {
	nextNoA := handler.WrapExecutable(&dummyNext{
		returnA:    false,
		returnAAAA: true,
	})
	nextNoAAAA := handler.WrapExecutable(&dummyNext{
		returnA:    true,
		returnAAAA: false,
	})
	nextDual := handler.WrapExecutable(&dummyNext{
		returnA:    true,
		returnAAAA: true,
	})
	nextLateA := handler.WrapExecutable(&dummyNext{
		returnA:    true,
		latencyA:   time.Millisecond * 100,
		returnAAAA: true,
	})
	nextLateAAAA := handler.WrapExecutable(&dummyNext{
		returnA:     true,
		returnAAAA:  true,
		latencyAAAA: time.Millisecond * 100,
	})

	tests := []struct {
		name      string
		mode      int
		qtype     uint16
		next      handler.ExecutableChainNode
		wantErr   bool
		wantReply bool
	}{
		{
			name:      "prefer v4: do not block domain AAAA if domain does not have an A record",
			mode:      modePreferIPv4,
			qtype:     dns.TypeAAAA,
			next:      nextNoA,
			wantErr:   false,
			wantReply: true,
		},
		{
			name:      "prefer v4: do not block domain AAAA if A reply wasn't returned on time",
			mode:      modePreferIPv4,
			qtype:     dns.TypeAAAA,
			next:      nextLateA,
			wantErr:   false,
			wantReply: true,
		},
		{
			name:      "prefer v4: block domain AAAA if domain has A records",
			mode:      modePreferIPv4,
			qtype:     dns.TypeAAAA,
			next:      nextDual,
			wantErr:   false,
			wantReply: false,
		},
		{
			name:      "prefer v6: do not block domain A if domain does not have an AAAA record",
			mode:      modePreferIPv6,
			qtype:     dns.TypeA,
			next:      nextNoAAAA,
			wantErr:   false,
			wantReply: true,
		},
		{
			name:      "prefer v6: do not block domain A if AAAA reply wasn't returned on time",
			mode:      modePreferIPv6,
			qtype:     dns.TypeA,
			next:      nextLateAAAA,
			wantErr:   false,
			wantReply: true,
		},
		{
			name:      "prefer v6: block domain A if domain has AAAA records",
			mode:      modePreferIPv6,
			qtype:     dns.TypeA,
			next:      nextDual,
			wantErr:   false,
			wantReply: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Selector{
				BP:          handler.NewBP("", PluginType),
				mode:        tt.mode,
				waitTimeout: time.Millisecond * 20,
			}

			q := new(dns.Msg)
			q.SetQuestion("example.", tt.qtype)
			qCtx := handler.NewContext(q, nil)
			if err := s.Exec(context.Background(), qCtx, tt.next); (err != nil) != tt.wantErr {
				t.Errorf("Exec() error = %v, wantErr %v", err, tt.wantErr)
			}

			r := qCtx.R()
			if hasReply := msgAnsHasRR(r, tt.qtype); hasReply != tt.wantReply {
				t.Errorf("Exec() hasReply = %v, wantReply %v", hasReply, tt.wantReply)
			}
		})
	}
}
