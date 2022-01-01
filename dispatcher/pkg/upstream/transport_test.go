package upstream

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestTransport_Exchange(t *testing.T) {
	dial := func(ctx context.Context) (net.Conn, error) {
		c1, c2 := net.Pipe()
		go func() {
			for {
				m, _, readErr := dnsutils.ReadMsgFromTCP(c2)
				if m != nil {
					go func() {
						time.Sleep(time.Millisecond * 50)
						dnsutils.WriteMsgToTCP(c2, m)
					}()
				}
				if readErr != nil {
					return
				}
			}
		}()
		return c1, nil
	}

	dialErr := func(ctx context.Context) (net.Conn, error) {
		return nil, errors.New("dial err")
	}

	write := dnsutils.WriteRawMsgToTCP

	read := dnsutils.ReadRawMsgFromTCP

	writeErr := func(c io.Writer, m []byte) (n int, err error) {
		return 0, errors.New("write err")
	}

	readErr := func(c io.Reader) (m []byte, n int, err error) {
		return nil, 0, errors.New("read err")
	}

	readTimeout := func(c io.Reader) (m []byte, n int, err error) {
		time.Sleep(time.Second * 10)
		return nil, 0, errors.New("read err")
	}

	type fields struct {
		DialFunc    func(ctx context.Context) (net.Conn, error)
		WriteFunc   func(c io.Writer, m []byte) (n int, err error)
		ReadFunc    func(c io.Reader) (m []byte, n int, err error)
		MaxConns    int
		IdleTimeout time.Duration
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
			name: "connection reuse",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    read,
				IdleTimeout: time.Millisecond * 100,
				MaxConns:    5,
			},
			N:       32,
			wantErr: false,
		},
		{
			name: "dial err",
			fields: fields{
				DialFunc:    dialErr,
				WriteFunc:   write,
				ReadFunc:    read,
				IdleTimeout: time.Millisecond * 100,
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
				MaxQueryPerConn: 8,
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
					q := new(dns.Msg)
					qName := fmt.Sprintf("%d.", i)
					q.SetQuestion(qName, dns.TypeA)
					raw, err := q.Pack()
					if err != nil {
						t.Error(err)
						return
					}
					ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
					defer cancel()

					gotR, err := transport.ExchangeContext(ctx, raw)
					if (err != nil) != tt.wantErr {
						t.Errorf("Exchange() error = %v, wantErr %v", err, tt.wantErr)
						return
					}

					r := new(dns.Msg)
					if err := r.Unpack(gotR); err != nil {
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
