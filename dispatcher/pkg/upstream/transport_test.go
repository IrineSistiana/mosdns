package upstream

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/pool"
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
	randSleep20 := func() { time.Sleep(time.Millisecond * time.Duration(rand.Intn(20))) }
	dial := func(ctx context.Context) (net.Conn, error) {
		c1, c2 := net.Pipe()
		go func() {
			for {
				m, _, readErr := dnsutils.ReadRawMsgFromTCP(c2)
				if m != nil {
					go func() {
						defer m.Release()
						randSleep20()
						dnsutils.WriteRawMsgToTCP(c2, m.Bytes())
					}()
				}
				if readErr != nil {
					return
				}
			}
		}()
		randSleep20()
		return c1, nil
	}

	dialErr := func(ctx context.Context) (net.Conn, error) {
		return nil, errors.New("dial err")
	}

	write := func(c io.Writer, m []byte) (n int, err error) {
		randSleep20()
		return dnsutils.WriteRawMsgToTCP(c, m)
	}

	read := func(c io.Reader) (m *pool.Buffer, n int, err error) {
		randSleep20()
		return dnsutils.ReadRawMsgFromTCP(c)
	}

	writeErr := func(c io.Writer, m []byte) (n int, err error) {
		return 0, errors.New("write err")
	}

	readErr := func(c io.Reader) (m *pool.Buffer, n int, err error) {
		return nil, 0, errors.New("read err")
	}

	readTimeout := func(c io.Reader) (m *pool.Buffer, n int, err error) {
		time.Sleep(time.Second * 1)
		return nil, 0, errors.New("read err")
	}

	type fields struct {
		DialFunc        func(ctx context.Context) (net.Conn, error)
		WriteFunc       func(c io.Writer, m []byte) (n int, err error)
		ReadFunc        func(c io.Reader) (m *pool.Buffer, n int, err error)
		MaxConns        int
		IdleTimeout     time.Duration
		DisablePipeline bool
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
				IdleTimeout: 0,
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
				IdleTimeout: 0,
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
				IdleTimeout: 0,
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
				IdleTimeout: 0,
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
				IdleTimeout: 0,
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
				IdleTimeout: time.Millisecond * 100,
				MaxConns:    256,
			},
			N:       512,
			wantErr: false,
		},
		{
			name: "dial err",
			fields: fields{
				DialFunc:    dialErr,
				WriteFunc:   write,
				ReadFunc:    read,
				IdleTimeout: time.Millisecond * 100,
				MaxConns:    16,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "write err",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   writeErr,
				ReadFunc:    read,
				IdleTimeout: time.Millisecond * 100,
				MaxConns:    16,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "read err",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    readErr,
				IdleTimeout: time.Millisecond * 100,
				MaxConns:    16,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "read timeout",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    readTimeout,
				IdleTimeout: time.Millisecond * 100,
				MaxConns:    16,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "np",
			fields: fields{
				DialFunc:        dial,
				WriteFunc:       write,
				ReadFunc:        read,
				IdleTimeout:     time.Millisecond * 100,
				DisablePipeline: true,
			},
			N:       512,
			wantErr: false,
		},
		{
			name: "np dial err",
			fields: fields{
				DialFunc:        dialErr,
				WriteFunc:       write,
				ReadFunc:        read,
				IdleTimeout:     time.Millisecond * 100,
				DisablePipeline: true,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "np write err",
			fields: fields{
				DialFunc:        dial,
				WriteFunc:       writeErr,
				ReadFunc:        read,
				IdleTimeout:     time.Millisecond * 100,
				DisablePipeline: true,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "np read err",
			fields: fields{
				DialFunc:        dial,
				WriteFunc:       write,
				ReadFunc:        readErr,
				IdleTimeout:     time.Millisecond * 100,
				DisablePipeline: true,
			},
			N:       16,
			wantErr: true,
		},
		{
			name: "np read timeout",
			fields: fields{
				DialFunc:        dial,
				WriteFunc:       write,
				ReadFunc:        readTimeout,
				IdleTimeout:     time.Millisecond * 100,
				DisablePipeline: true,
			},
			N:       16,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &Transport{
				Logger:          zap.NewNop(),
				DialFunc:        tt.fields.DialFunc,
				WriteFunc:       tt.fields.WriteFunc,
				ReadFunc:        tt.fields.ReadFunc,
				MaxConns:        tt.fields.MaxConns,
				IdleTimeout:     tt.fields.IdleTimeout,
				DisablePipeline: tt.fields.DisablePipeline,
				MaxQueryPerConn: 2,
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
			defer cancel()

			if tt.wantErr {
				q := new(dns.Msg)
				q.SetQuestion(".", dns.TypeA)
				raw, err := q.Pack()
				if err != nil {
					t.Fatal(err)
				}
				_, err = transport.ExchangeContext(ctx, raw)
				if err == nil {
					t.Fatal("want err, but got nil err")
				}
				return
			}
			wg := new(sync.WaitGroup)
			for i := 0; i < tt.N; i++ {
				wg.Add(1)
				i := i
				go func() {
					defer wg.Done()
					randSleep20()
					q := new(dns.Msg)
					qName := fmt.Sprintf("%d.", i)
					q.SetQuestion(qName, dns.TypeA)
					raw, err := q.Pack()
					if err != nil {
						t.Error(err)
						return
					}
					ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
					defer cancel()

					gotR, err := transport.ExchangeContext(ctx, raw)
					if (err != nil) != tt.wantErr {
						t.Errorf("Exchange() error = %v, wantErr %v", err, tt.wantErr)
						return
					}

					r := new(dns.Msg)
					if err := r.Unpack(gotR.Bytes()); err != nil {
						t.Error(err)
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
			time.Sleep(tt.fields.IdleTimeout + time.Millisecond*100)

			transport.cm.Lock()
			if n := len(transport.clientConns); n != 0 {
				t.Errorf("len(t.clientConns), want 0, got %d", n)
			}

			if n := len(transport.dCalls); n != 0 {
				t.Errorf("len(t.clientConns), want 0, got %d", n)
			}
			transport.cm.Unlock()
		})
	}
}
