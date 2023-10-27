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

package transport

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

type dummyEchoNetConn struct {
	net.Conn
	rErrProb float64
	rLatency time.Duration
	wErrProb float64

	closeOnce   sync.Once
	closeNotify chan struct{}
}

func newDummyEchoNetConn(rErrProb float64, rLatency time.Duration, wErrProb float64) NetConn {
	c1, c2 := net.Pipe()
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer c1.Close()
		defer c2.Close()
		for {
			m, readErr := dnsutils.ReadRawMsgFromTCP(c2)
			if m != nil {
				go func() {
					defer pool.ReleaseBuf(m)
					if rLatency > 0 {
						t := time.NewTimer(rLatency)
						defer t.Stop()
						select {
						case <-t.C:
						case <-ctx.Done():
							return
						}
					}
					latency := time.Millisecond * time.Duration(rand.Intn(20))
					time.Sleep(latency)
					_, _ = dnsutils.WriteRawMsgToTCP(c2, *m)
				}()
			}
			if readErr != nil {
				return
			}
		}
	}()
	return &dummyEchoNetConn{
		Conn:        c1,
		rErrProb:    rErrProb,
		rLatency:    rLatency,
		wErrProb:    wErrProb,
		closeNotify: make(chan struct{}),
	}
}

func probTrue(p float64) bool {
	return rand.Float64() < p
}

func (d *dummyEchoNetConn) Read(p []byte) (n int, err error) {
	if probTrue(d.rErrProb) {
		return 0, errors.New("read err")
	}
	return d.Conn.Read(p)
}

func (d *dummyEchoNetConn) Write(p []byte) (n int, err error) {
	if probTrue(d.wErrProb) {
		return 0, errors.New("write err")
	}
	return d.Conn.Write(p)
}

func (d *dummyEchoNetConn) Close() error {
	d.closeOnce.Do(func() {
		close(d.closeNotify)
	})
	return d.Conn.Close()
}

func Test_dnsConn_exchange(t *testing.T) {
	idleTimeout := time.Millisecond * 100

	tests := []struct {
		name       string
		rErrProb   float64
		rLatency   time.Duration
		wErrProb   float64
		connClosed bool // connection is closed before calling exchange()
		wantMsg    bool
		wantErr    bool
	}{
		{
			name:     "normal",
			rErrProb: 0,
			rLatency: 0,
			wErrProb: 0,
			wantMsg:  true, wantErr: false,
		},
		{
			name:     "write err",
			rErrProb: 0,
			rLatency: 0,
			wErrProb: 1,
			wantMsg:  false, wantErr: true,
		},
		{
			name:     "read err",
			rErrProb: 1,
			rLatency: 0,
			wErrProb: 0,
			wantMsg:  false, wantErr: true,
		},
		{
			name:     "read timeout",
			rErrProb: 0,
			rLatency: idleTimeout * 3,
			wErrProb: 0,
			wantMsg:  false, wantErr: true,
		},
		{
			name:       "connection closed",
			rErrProb:   0,
			rLatency:   0,
			wErrProb:   0,
			connClosed: true,
			wantMsg:    false, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := require.New(t)
			c := newDummyEchoNetConn(tt.rErrProb, tt.rLatency, tt.wErrProb)
			defer c.Close()
			ioOpts := TraditionalDnsConnOpts{
				WithLengthHeader: true, // TODO: Test false as well
				IdleTimeout:      idleTimeout,
			}
			dc := NewDnsConn(ioOpts, c)

			if tt.connClosed {
				dc.Close()
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
			defer cancel()
			q := new(dns.Msg)
			q.SetQuestion("test.", dns.TypeA)
			queryPayload, err := q.Pack()
			r.NoError(err)

			rec, closed := dc.ReserveNewQuery()
			r.Equal(tt.connClosed, closed)
			if rec == nil {
				return
			}

			respPayload, err := rec.ExchangeReserved(ctx, queryPayload)
			if tt.wantErr {
				r.Error(err)
			} else {
				r.NoError(err)
			}

			if tt.wantMsg {
				r.NotNil(respPayload)
				r.True(bytes.Equal(queryPayload, *respPayload))

				// test idle timeout
				time.Sleep(idleTimeout + time.Millisecond*20)
				runtime.Gosched()
				r.True(dc.IsClosed(), "connection should be closed due to idle timeout")
			} else {
				r.Nil(respPayload)
			}
		})
	}
}

// TODO: 测试 maxconcurrentquery。

func Test_dnsConn_exchange_race(t *testing.T) {
	r := require.New(t)
	wg := new(sync.WaitGroup)
	for i := 0; i < 1024; i++ {
		c := newDummyEchoNetConn(0.5, time.Millisecond*20, 0.5)
		ioOpts := TraditionalDnsConnOpts{
			WithLengthHeader: true, // TODO: Test false as well
			IdleTimeout:      time.Millisecond * 50,
		}
		dc := NewDnsConn(ioOpts, c)
		for j := 0; j < 24; j++ {
			if dc.IsClosed() {
				break
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
				defer cancel()
				q := new(dns.Msg)
				q.SetQuestion("test.", dns.TypeA)
				queryPayload, err := q.Pack()
				r.NoError(err)

				rec, closed := dc.ReserveNewQuery()
				if closed {
					return
				}
				if rec != nil {
					_, _ = rec.ExchangeReserved(ctx, queryPayload)
				}
			}()
		}
	}
	wg.Wait()
}
