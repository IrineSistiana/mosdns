package transport

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/quic-go/quic-go"
)

var _ DnsConn = (*QuicDnsConn)(nil)

type QuicDnsConn struct {
	c quic.Connection
}

func NewQuicDnsConn(c quic.Connection) *QuicDnsConn {
	return &QuicDnsConn{c: c}
}

func (c *QuicDnsConn) Close() error {
	return c.c.CloseWithError(0, "")
}

func (c *QuicDnsConn) ReserveNewQuery() (_ ReservedExchanger, closed bool) {
	select {
	case <-c.c.Context().Done():
		return nil, true
	default:
	}
	s, err := c.c.OpenStream()
	// We just checked the connection is alive. So we are assuming the error
	// is caused by reaching the peer's stream limit.
	if err != nil {
		return nil, false
	}
	return &quicReservedExchanger{stream: s}, false
}

type quicReservedExchanger struct {
	stream quic.Stream
}

var _ ReservedExchanger = (*quicReservedExchanger)(nil)

func (ote *quicReservedExchanger) ExchangeReserved(ctx context.Context, q []byte) (resp *[]byte, err error) {
	defer ote.WithdrawReserved()

	payload, err := copyMsgWithLenHdr(q)
	if err != nil {
		return nil, err
	}

	// 4.2.1.  DNS Message IDs
	//    When sending queries over a QUIC connection, the DNS Message ID MUST
	//    be set to 0.  The stream mapping for DoQ allows for unambiguous
	//    correlation of queries and responses, so the Message ID field is not
	//    required.
	orgQid := binary.BigEndian.Uint16((*payload)[2:])
	binary.BigEndian.PutUint16((*payload)[2:], 0)

	// TODO: use single goroutine.
	// See RFC9250 4.3.1. Transaction Cancellation
	type res struct {
		resp *[]byte
		err  error
	}
	rc := make(chan res, 1)
	go func() {
		defer func() {
			pool.ReleaseBuf(payload)
		}()
		r, err := exchangeTroughQuicStream(ote.stream, *payload)
		rc <- res{resp: r, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case r := <-rc:
		resp := r.resp
		err := r.err
		if resp != nil {
			binary.BigEndian.PutUint16((*resp), orgQid)
		}
		return resp, err
	}
}

func (ote *quicReservedExchanger) WithdrawReserved() {
	s := ote.stream
	s.Close()
	s.CancelRead(0) // TODO: Needs a proper error code.
}

func exchangeTroughQuicStream(s quic.Stream, payload []byte) (*[]byte, error) {
	s.SetDeadline(time.Now().Add(quicQueryTimeout))

	_, err := s.Write(payload)
	if err != nil {
		return nil, err
	}

	// RFC 9250 4.2
	//    The client MUST send the DNS query over the selected stream and MUST
	//    indicate through the STREAM FIN mechanism that no further data will
	//    be sent on that stream.
	//
	// Call Close() here will send the STREAM FIN. It won't close Read.
	s.Close()
	return dnsutils.ReadRawMsgFromTCP(s)
}
