package transport

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type lazyDnsConn struct {
	maxConcurrentQuery int
	cancelDial         context.CancelFunc

	mu                 sync.Mutex
	earlyReserveCallWg sync.WaitGroup
	closed             bool
	reservedQuery      int
	dialFinished       chan struct{}
	c                  DnsConn
	dialErr            error

	// 1: dial completed and all early reserve call finished.
	// 2: dial failed.
	fastPath atomic.Uint32
}

var _ DnsConn = (*lazyDnsConn)(nil)

var (
	errLazyConnDialCanceled = errors.New("lazy dial canceled")
)

func newLazyDnsConn(
	dial func(ctx context.Context) (DnsConn, error),
	dialTimeout time.Duration,
	maxConcurrentQueryWhileDialing int, // must be valid, no default value
	logger *zap.Logger, // must non-nil
) *lazyDnsConn {
	if dialTimeout <= 0 {
		dialTimeout = defaultDialTimeout
	}
	dialCtx, cancelDial := context.WithTimeout(context.Background(), defaultDialTimeout)
	lc := &lazyDnsConn{
		maxConcurrentQuery: maxConcurrentQueryWhileDialing,
		cancelDial:         cancelDial,
		dialFinished:       make(chan struct{}),
	}

	go func() {
		dc, err := dial(dialCtx)
		cancelDial()
		if err != nil {
			logger.Check(zap.WarnLevel, "failed to dial dns conn").Write(zap.Error(err))
		}
		lc.mu.Lock()
		if lc.closed { // lc was closed and dial was canceled
			lc.mu.Unlock()
			if dc != nil {
				dc.Close()
			}
			return
		}

		lc.c = dc
		lc.dialErr = err
		close(lc.dialFinished)
		lc.mu.Unlock()
	}()
	return lc
}

func (lc *lazyDnsConn) Close() error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.closed {
		return nil
	}
	lc.closed = true

	if lc.c == nil && lc.dialErr == nil { // still dialing
		lc.cancelDial()
		lc.dialErr = errLazyConnDialCanceled
		close(lc.dialFinished)
	} else {
		// close connection
		if lc.c != nil {
			lc.c.Close()
		}
	}
	return nil
}

func (lc *lazyDnsConn) ReserveNewQuery() (_ ReservedExchanger, closed bool) {
	switch lc.fastPath.Load() {
	case 1:
		return lc.c.ReserveNewQuery()
	case 2:
		return nil, true
	}

	lc.mu.Lock()
	defer lc.mu.Unlock()

	select {
	case <-lc.dialFinished:
		// Note: race condition here and lazyDnsConnEarlyReservedExchanger.ExchangeReserved().
		// Not a big problem. May cause at most all early exchange failed.
		// earlyExchangeWg makes sure that early exchange calls ReserveNewQuery first.
		dc, err := lc.c, lc.dialErr
		if err != nil {
			lc.fastPath.Store(2)
			return nil, true
		}
		lc.earlyReserveCallWg.Wait()
		lc.fastPath.Store(1)
		return dc.ReserveNewQuery()
	default:
		if lc.reservedQuery >= lc.maxConcurrentQuery {
			return nil, false
		}
		lc.reservedQuery++
		lc.earlyReserveCallWg.Add(1)
		return (*lazyDnsConnEarlyReservedExchanger)(lc), false
	}
}

type lazyDnsConnEarlyReservedExchanger lazyDnsConn

var _ ReservedExchanger = (*lazyDnsConnEarlyReservedExchanger)(nil)

func (ote *lazyDnsConnEarlyReservedExchanger) ExchangeReserved(ctx context.Context, q []byte) (resp *[]byte, err error) {
	defer func() {
		ote.mu.Lock()
		ote.reservedQuery--
		ote.mu.Unlock()
	}()

	select {
	case <-ctx.Done():
		ote.earlyReserveCallWg.Done()
		return nil, context.Cause(ctx)
	case <-ote.dialFinished:
		dc, err := ote.c, ote.dialErr
		if err != nil {
			return nil, err
		}
		rec, _ := dc.ReserveNewQuery()
		ote.earlyReserveCallWg.Done()
		if rec == nil {
			return nil, ErrLazyConnCannotReserveQueryExchanger
		}
		return rec.ExchangeReserved(ctx, q)
	}
}

func (ote *lazyDnsConnEarlyReservedExchanger) WithdrawReserved() {
	ote.earlyReserveCallWg.Done()
	ote.mu.Lock()
	ote.reservedQuery--
	ote.mu.Unlock()
}
