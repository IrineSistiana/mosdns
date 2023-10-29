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
	"fmt"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/dnsutils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"
)

func TestTransport_Exchange(t *testing.T) {
	randSleepMs := func(ms int) { time.Sleep(time.Millisecond * time.Duration(rand.Intn(ms))) }
	dial := func(ctx context.Context) (net.Conn, error) {
		c1, c2 := net.Pipe()
		go func() {
			for {
				m, _, readErr := dnsutils.ReadRawMsgFromTCP(c2)
				if m != nil {
					go func() {
						defer m.Release()
						randSleepMs(20)
						dnsutils.WriteRawMsgToTCP(c2, m.Bytes())
					}()
				}
				if readErr != nil {
					return
				}
			}
		}()
		randSleepMs(20)
		return c1, nil
	}

	dialErr := func(ctx context.Context) (net.Conn, error) {
		return nil, errors.New("dial err")
	}

	write := func(c io.Writer, m *dns.Msg) (n int, err error) {
		randSleepMs(20)
		return dnsutils.WriteMsgToTCP(c, m)
	}

	read := func(c io.Reader) (m *dns.Msg, n int, err error) {
		randSleepMs(20)
		return dnsutils.ReadMsgFromTCP(c)
	}

	writeErr := func(c io.Writer, m *dns.Msg) (n int, err error) {
		return 0, errors.New("write err")
	}

	readErr := func(c io.Reader) (m *dns.Msg, n int, err error) {
		return nil, 0, errors.New("read err")
	}

	readTimeout := func(c io.Reader) (m *dns.Msg, n int, err error) {
		time.Sleep(time.Second * 1)
		return nil, 0, errors.New("read err")
	}

	type fields struct {
		DialFunc       func(ctx context.Context) (net.Conn, error)
		WriteFunc      func(c io.Writer, m *dns.Msg) (n int, err error)
		ReadFunc       func(c io.Reader) (m *dns.Msg, n int, err error)
		MaxConns       int
		IdleTimeout    time.Duration
		EnablePipeline bool
	}

	tests := []struct {
		name    string
		fields  fields
		N       int
		wantErr bool
	}{
		{
			name: "no connection reuse",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    read,
				IdleTimeout: -1,
			},
			N:       16,
			wantErr: false,
		},
		{
			name: "no connection reuse, dial err",
			fields: fields{
				DialFunc:    dialErr,
				WriteFunc:   write,
				ReadFunc:    read,
				IdleTimeout: -1,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "no connection reuse, write err",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   writeErr,
				ReadFunc:    read,
				IdleTimeout: -1,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "no connection reuse, read err",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    readErr,
				IdleTimeout: -1,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "no connection reuse, read timeout",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    readTimeout,
				IdleTimeout: -1,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "connection reuse",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    read,
				IdleTimeout: time.Millisecond * 200,
			},
			N:       32,
			wantErr: false,
		},
		{
			name: "connection reuse dial err",
			fields: fields{
				DialFunc:    dialErr,
				WriteFunc:   write,
				ReadFunc:    read,
				IdleTimeout: time.Millisecond * 200,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "connection reuse write err",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   writeErr,
				ReadFunc:    read,
				IdleTimeout: time.Millisecond * 200,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "connection reuse read err",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    readErr,
				IdleTimeout: time.Millisecond * 200,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "connection reuse read timeout",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    readTimeout,
				IdleTimeout: time.Millisecond * 200,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "pipeline connection reuse",
			fields: fields{
				DialFunc:       dial,
				WriteFunc:      write,
				ReadFunc:       read,
				IdleTimeout:    time.Millisecond * 100,
				EnablePipeline: true,
				MaxConns:       16,
			},
			N:       32,
			wantErr: false,
		},
		{
			name: "pipeline connection reuse dial err",
			fields: fields{
				DialFunc:       dialErr,
				WriteFunc:      write,
				ReadFunc:       read,
				IdleTimeout:    time.Millisecond * 100,
				EnablePipeline: true,
				MaxConns:       16,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "pipeline connection reuse write err",
			fields: fields{
				DialFunc:       dial,
				WriteFunc:      writeErr,
				ReadFunc:       read,
				IdleTimeout:    time.Millisecond * 100,
				EnablePipeline: true,
				MaxConns:       16,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "pipeline connection reuse read err",
			fields: fields{
				DialFunc:       dial,
				WriteFunc:      write,
				ReadFunc:       readErr,
				IdleTimeout:    time.Millisecond * 100,
				EnablePipeline: true,
				MaxConns:       16,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "pipeline connection reuse read timeout",
			fields: fields{
				DialFunc:       dial,
				WriteFunc:      write,
				ReadFunc:       readTimeout,
				IdleTimeout:    time.Millisecond * 100,
				EnablePipeline: true,
				MaxConns:       16,
			},
			N:       16,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Opts{
				Logger:          zap.NewNop(),
				DialFunc:        tt.fields.DialFunc,
				WriteFunc:       tt.fields.WriteFunc,
				ReadFunc:        tt.fields.ReadFunc,
				MaxConns:        tt.fields.MaxConns,
				IdleTimeout:     tt.fields.IdleTimeout,
				EnablePipeline:  tt.fields.EnablePipeline,
				MaxQueryPerConn: 2,
			}
			transport, err := NewTransport(opts)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
			defer cancel()

			// Test err first.
			if tt.wantErr {
				q := new(dns.Msg)
				q.SetQuestion(".", dns.TypeA)
				_, err := transport.ExchangeContext(ctx, q)
				if err == nil {
					t.Fatal("want err, but got nil err")
				}
				return
			}

			// Concurrent test.
			wg := new(sync.WaitGroup)
			for i := 0; i < tt.N; i++ {
				wg.Add(1)
				i := i
				go func() {
					defer wg.Done()
					randSleepMs(100)
					q := new(dns.Msg)
					qName := fmt.Sprintf("%d.", i)
					q.SetQuestion(qName, dns.TypeA)
					ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
					defer cancel()

					r, err := transport.ExchangeContext(ctx, q)
					if (err != nil) != tt.wantErr {
						t.Errorf("Exchange() error = %v, wantErr %v", err, tt.wantErr)
						return
					}

					if gotName := r.Question[0].Name; gotName != qName {
						t.Errorf("Exchange() gotName = %v, want %v", gotName, qName)
					}
				}()

				if t.Failed() {
					break
				}
			}
			wg.Wait()

			// Wait until all connections are timed out.
			time.Sleep(tt.fields.IdleTimeout + time.Millisecond*200)

			_, _, newConn, pipelineWg, err := transport.getPipelineConn()
			if err != nil {
				t.Fatal(err)
			}
			if !newConn {
				t.Fatal("pipelineConn should be a new connection")
			}
			pipelineWg.Done()
			reusableConn, reused, err := transport.getReusableConn()
			if err != nil {
				t.Fatal(err)
			}
			if reused {
				t.Fatal("reusableConn should be a new connection")
			}
			transport.releaseReusableConn(reusableConn, nil)

			if n := len(transport.idledReusableConns); n != 1 {
				t.Errorf("len(t.idledReusableConns), want 1, got %d", n)
			}
			if n := len(transport.reusableConns); n != 1 {
				t.Errorf("len(t.reusableConns), want 1, got %d", n)
			}
			if n := len(transport.pipelineConns); n != 1 {
				t.Errorf("len(t.pipelineConns), want 1, got %d", n)
			}
		})
	}
}
