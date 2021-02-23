package upstream

import (
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"io"
	"net"
	"testing"
	"time"
)

type dummyV struct {
	id uint64
}

func TestTransport_Exchange(t1 *testing.T) {
	dial := func() (net.Conn, error) {
		c1, c2 := net.Pipe()
		go func() {
			buf := make([]byte, 16)
			for {
				n, readErr := c2.Read(buf)
				if n > 0 {
					_, writeErr := c2.Write(buf[:n])
					if writeErr != nil {
						return
					}
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
			wantErr: false,
		},
		{
			name: "connection reuse",
			fields: fields{
				DialFunc:    dial,
				WriteFunc:   write,
				ReadFunc:    read,
				IdleTimeout: time.Millisecond * 100,
				MaxConns:    2,
			},
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
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transport{
				Logger:      zap.NewNop(),
				DialFunc:    tt.fields.DialFunc,
				WriteFunc:   tt.fields.WriteFunc,
				ReadFunc:    tt.fields.ReadFunc,
				MaxConns:    tt.fields.MaxConns,
				IdleTimeout: tt.fields.IdleTimeout,
				Timeout:     time.Millisecond * 100,
			}

			if tt.wantErr {
				_, _, err := t.Exchange(&dns.Msg{})
				if err == nil {
					t1.Fatal("want err, but got nil err")
				}
				return
			}

			for id := uint16(0); id < 16; id++ {
				v := &dns.Msg{}
				v.Id = id
				gotR, _, err := t.Exchange(v)
				if (err != nil) != tt.wantErr {
					t1.Errorf("Exchange() error = %v, wantErr %v", err, tt.wantErr)
					return
				}

				if gotR.Id != id {
					t1.Errorf("Exchange() gotR.Id = %v, want %v", gotR.Id, id)
				}
			}

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
