package blackhole

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"testing"
)

func Test_blackhole_Do(t *testing.T) {
	tests := []struct {
		name         string
		argsRcode    int
		wantRcode    int
		wantResponse bool
	}{
		{"Drop response", 0, 0, false},
		{"Respond with rcode 2", 2, 2, true},
		{"Respond with rcode 3", 3, 3, true},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &blackhole{
				rCode: tt.argsRcode,
			}
			qCtx := new(handler.Context)
			q := new(dns.Msg)
			q.SetQuestion("exmaple.com", dns.TypeA)
			r := new(dns.Msg)
			r.SetReply(q)
			qCtx.Q = q
			qCtx.R = r

			err := b.Do(ctx, qCtx)
			if err != nil {
				t.Fatal(err)
			}

			if !tt.wantResponse && qCtx.R != nil {
				t.Error("response should be droped")
			}

			if tt.wantResponse {
				if tt.wantRcode != qCtx.R.Rcode {
					t.Errorf("response should have rcode %d, but got %d", tt.wantRcode, qCtx.R.Rcode)
				}
			}
		})
	}
}
