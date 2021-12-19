package upstream

import (
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

func TestTransport_Exchange(t1 *testing.T) {
	dial := func() (net.Conn, error) {
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

	dialErr := func() (net.Conn, error) {
		return nil, errors.New("dial err")
	}

	write := dnsutils.WriteMsgToTCP

	read := dnsutils.ReadMsgFromTCP

	writeErr := func(c io.Writer, v *dns.Msg) (n int, err error) {
		return 0, errors.New("write err")
	}

	readErr := func(c io.Reader) (v *dns.Msg, n int, err error) {
		return nil, 0, errors.New("read err")
	}

	readTimeout := func(c io.Reader) (v *dns.Msg, n int, err error) {
		time.Sleep(time.Second * 10)
		return nil, 0, errors.New("read err")
	}

	type fields struct {
		DialFunc    func() (net.Conn, error)
		WriteFunc   func(c io.Writer, v *dns.Msg) (n int, err error)
		ReadFunc    func(c io.Reader) (v *dns.Msg, n int, err error)
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
				IdleTimeout: time.Millisecond * 1000,
				MaxConns:    2,
			},
			N:       16,
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
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transport{
				Logger:          zap.NewNop(),
				DialFunc:        tt.fields.DialFunc,
				WriteFunc:       tt.fields.WriteFunc,
				ReadFunc:        tt.fields.ReadFunc,
				MaxConns:        tt.fields.MaxConns,
				IdleTimeout:     tt.fields.IdleTimeout,
				MaxQueryPerConn: 8,
				Timeout:         time.Millisecond * 100,
			}

			if tt.wantErr {
				_, err := t.Exchange(&dns.Msg{})
				if err == nil {
					t1.Fatal("want err, but got nil err")
				}
				return
			}
			wg := new(sync.WaitGroup)
			for i := 0; i < tt.N; i++ {

				wg.Add(1)
				i := i
				go func() {
					defer wg.Done()
					v := new(dns.Msg)
					qName := fmt.Sprintf("%d.", i)
					v.SetQuestion(qName, dns.TypeA)
					gotR, err := t.Exchange(v)
					if (err != nil) != tt.wantErr {
						t1.Errorf("Exchange() error = %v, wantErr %v", err, tt.wantErr)
						return
					}

					if gotName := gotR.Question[0].Name; gotName != qName {
						t1.Errorf("Exchange() gotName = %v, want %v", gotName, qName)
					}
				}()

				if t1.Failed() {
					break
				}
			}

			wg.Wait()
			time.Sleep(tt.fields.IdleTimeout + time.Millisecond*200)

			t.cm.Lock()
			if n := len(t.conns); n != 0 {
				t1.Errorf("len(t.conns), want 0, got %d", n)
			}

			if n := len(t.dialingCalls); n != 0 {
				t1.Errorf("len(t.conns), want 0, got %d", n)
			}
			t.cm.Unlock()
		})
	}
}
