package single_flight

import (
	"context"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/miekg/dns"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type dummyNext struct {
	c uint32
}

func (d *dummyNext) Exec(ctx context.Context, qCtx *handler.Context, _ handler.ExecutableChainNode) error {
	atomic.AddUint32(&d.c, 1)
	<-ctx.Done()
	r := new(dns.Msg)
	r.SetReply(qCtx.Q())
	qCtx.SetResponse(r, handler.ContextStatusResponded)
	return nil
}

func TestSingleFlight_Exec(t *testing.T) {
	sfg := new(SingleFlight)

	var nextNodes []handler.ExecutableChainNode
	for i := 0; i < 5; i++ {
		nextNode := handler.WrapExecutable(new(dummyNext))
		nextNodes = append(nextNodes, nextNode)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := new(sync.WaitGroup)
	for _, next := range nextNodes {
		for i := 0; i < 5; i++ {
			wg.Add(1)
			next := next
			go func() {
				defer wg.Done()
				m := new(dns.Msg)
				m.SetQuestion("example.", dns.TypeA)
				m.Id = dns.Id()
				qCtx := handler.NewContext(m, nil)
				if err := sfg.Exec(ctx, qCtx, next); err != nil {
					t.Errorf("wanted err: nil, but got: %v", err)
				}
				// check msg id
				if qCtx.Q().Id != qCtx.R().Id {
					t.Error("msg id mismatched")
				}
			}()
		}
	}

	time.Sleep(time.Millisecond * 100)
	cancel()
	wg.Wait()

	for _, next := range nextNodes {
		c := next.(*handler.ExecutableNodeWrapper).Executable.(*dummyNext).c
		if c != 1 {
			t.Fatalf("next was called %d times", c)
		}
	}

}
