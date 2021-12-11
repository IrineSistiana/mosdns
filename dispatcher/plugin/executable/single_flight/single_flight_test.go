package single_flight

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/miekg/dns"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Test_singleFlight_Exec(t *testing.T) {
	sf := &SingleFlightPlugin{
		BP: handler.NewBP("", ""),
	}

	dummyNext := &dummyEN{returnSignal: make(chan struct{})}

	wg := new(sync.WaitGroup)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			qCtx := handler.NewContext(new(dns.Msg), nil)
			err := sf.Exec(context.Background(), qCtx, handler.WrapExecutable(dummyNext))
			if err != nil {
				t.Error(err)
			}
			if id := qCtx.R().Id; id != 0 {
				t.Errorf("sf executed %d time(s)", id)
			}
		}()
	}

	time.Sleep(time.Millisecond * 100)
	close(dummyNext.returnSignal)
	wg.Wait()
}

func Test_singleFlight_Exec_err(t *testing.T) {
	sf := &SingleFlightPlugin{
		BP: handler.NewBP("", ""),
	}

	wantErr := errors.New("")
	dummyNext := &dummyEN{returnSignal: make(chan struct{}), wantErr: wantErr}

	wg := new(sync.WaitGroup)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			qCtx := handler.NewContext(new(dns.Msg), nil)
			err := sf.Exec(context.Background(), qCtx, handler.WrapExecutable(dummyNext))
			if err != wantErr {
				t.Error("sf returned an unexpected error")
			}
		}()
	}

	time.Sleep(time.Millisecond * 100)
	close(dummyNext.returnSignal)
	wg.Wait()
}

type dummyEN struct {
	returnSignal chan struct{}
	wantErr      error

	c uint32
}

func (d *dummyEN) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	id := atomic.LoadUint32(&d.c)
	defer atomic.AddUint32(&d.c, 1)

	r := new(dns.Msg)
	r.Id = uint16(id)
	qCtx.SetResponse(r, handler.ContextStatusResponded)

	if d.wantErr != nil {
		return d.wantErr
	}
	<-d.returnSignal
	return nil
}
