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
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/miekg/dns"
	"io"
	"math/rand"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

var (
	// dial a connection that has random 0~20ms response latency.
	dial = func(ctx context.Context) (net.Conn, error) {
		c1, c2 := net.Pipe()
		go func() {
			for {
				m, _, readErr := dnsutils.ReadRawMsgFromTCP(c2)
				if m != nil {
					go func() {
						defer pool.ReleaseBuf(m)
						latency := time.Millisecond * time.Duration(rand.Intn(20))
						time.Sleep(latency)
						_, _ = dnsutils.WriteRawMsgToTCP(c2, m)
					}()
				}
				if readErr != nil {
					return
				}
			}
		}()
		return c1, nil
	}

	dialErr = func(ctx context.Context) (net.Conn, error) {
		return nil, errors.New("dial err")
	}

	write = func(c io.Writer, m *dns.Msg) (n int, err error) {
		return dnsutils.WriteMsgToTCP(c, m)
	}

	read = func(c io.Reader) (m *dns.Msg, n int, err error) {
		return dnsutils.ReadMsgFromTCP(c)
	}

	writeErr = func(c io.Writer, m *dns.Msg) (n int, err error) {
		return 0, errors.New("write err")
	}

	readErr = func(c io.Reader) (m *dns.Msg, n int, err error) {
		return nil, 0, errors.New("read err")
	}

	slowRead1s = func(c io.Reader) (m *dns.Msg, n int, err error) {
		time.Sleep(time.Second * 1)
		return nil, 0, errors.New("read err")
	}

	dialErrP = func(ctx context.Context) (net.Conn, error) {
		if rand.Float64() < 0.5 {
			return dialErr(ctx)
		}
		return dial(ctx)
	}

	writeErrP = func(c io.Writer, m *dns.Msg) (n int, err error) {
		if rand.Float64() < 0.5 {
			return writeErr(c, m)
		}
		return write(c, m)
	}

	readErrP = func(c io.Reader) (m *dns.Msg, n int, err error) {
		if rand.Float64() < 0.5 {
			return readErr(c)
		}
		if rand.Float64() < 0.5 {
			return slowRead1s(c)
		}
		return read(c)
	}
)

func Test_dnsConn_exchange(t *testing.T) {
	tests := []struct {
		name    string
		ioOpts  IOOpts // set funcs only
		closed  bool   // connection is closed before calling exchange()
		wantMsg bool
		wantErr bool
	}{
		{
			name: "normal",
			ioOpts: IOOpts{
				DialFunc:  dial,
				WriteFunc: write,
				ReadFunc:  read,
			},
			wantMsg: true, wantErr: false,
		},
		{
			name: "dial err",
			ioOpts: IOOpts{
				DialFunc:  dialErr,
				WriteFunc: write,
				ReadFunc:  read,
			},
			wantMsg: false, wantErr: true,
		},
		{
			name: "write err",
			ioOpts: IOOpts{
				DialFunc:  dial,
				WriteFunc: writeErr,
				ReadFunc:  read,
			},
			wantMsg: false, wantErr: true,
		},
		{
			name: "read err",
			ioOpts: IOOpts{
				DialFunc:  dial,
				WriteFunc: write,
				ReadFunc:  readErr,
			},
			wantMsg: false, wantErr: true,
		},
		{
			name: "read timeout",
			ioOpts: IOOpts{
				DialFunc:  dial,
				WriteFunc: write,
				ReadFunc:  slowRead1s,
			},
			wantMsg: false, wantErr: true,
		},
		{
			name: "connection closed",
			ioOpts: IOOpts{
				DialFunc:  dial,
				WriteFunc: write,
				ReadFunc:  read,
			},
			closed:  true,
			wantMsg: false, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.ioOpts.IdleTimeout = time.Millisecond * 100
			dc := newDnsConn(tt.ioOpts)

			if tt.closed {
				dc.closeWithErr(net.ErrClosed)
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
			defer cancel()
			q := new(dns.Msg)
			q.SetQuestion("test.", dns.TypeA)
			r, err := dc.exchange(ctx, q)
			if (err != nil) != tt.wantErr {
				t.Fatalf("exchange() error = %v, wantErr %v", err, tt.wantErr)
			}

			if (r != nil) != tt.wantMsg {
				t.Fatalf("exchange() has msg: %v, wantMsg %v", r != nil, tt.wantMsg)
			}
			if tt.wantMsg {
				if r.Id != q.Id {
					t.Fatalf("invalid response, qid: %d, rid %d", q.Id, r.Id)
				}
				time.Sleep(tt.ioOpts.IdleTimeout + time.Millisecond*20)
				runtime.Gosched()
				if !dc.isClosed() {
					t.Fatalf("connection should have been closed due to idle timed out")
				}
			}
		})
	}
}

func Test_dnsConn_exchange_race(t *testing.T) {
	dc := &dnsConn{IOOpts: IOOpts{
		DialFunc:  dialErrP,
		WriteFunc: writeErrP,
		ReadFunc:  readErrP,
	}}

	wg := new(sync.WaitGroup)
	for i := 0; i < 1024; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
			defer cancel()
			q := new(dns.Msg)
			q.SetQuestion("test.", dns.TypeA)
			q.Id = uint16(i)
			_, _ = dc.exchange(ctx, q)
		}()
	}
	wg.Wait()
}
