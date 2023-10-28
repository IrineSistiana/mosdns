package transport

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/quic-go/quic-go"
)

const (
	quicQueryTimeout = time.Second * 6
)

const (
	// RFC 9250 4.3. DoQ Error Codes
	_DOQ_NO_ERROR          = quic.StreamErrorCode(0x0)
	_DOQ_INTERNAL_ERROR    = quic.StreamErrorCode(0x1)
	_DOQ_REQUEST_CANCELLED = quic.StreamErrorCode(0x3)
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
	stream := ote.stream

	payload, err := copyMsgWithLenHdr(q)
	if err != nil {
		stream.CancelWrite(_DOQ_REQUEST_CANCELLED)
		stream.CancelRead(_DOQ_REQUEST_CANCELLED)
		return nil, err
	}

	// 4.2.1.  DNS Message IDs
	//    When sending queries over a QUIC connection, the DNS Message ID MUST
	//    be set to 0.  The stream mapping for DoQ allows for unambiguous
	//    correlation of queries and responses, so the Message ID field is not
	//    required.
	orgQid := binary.BigEndian.Uint16((*payload)[2:])
	binary.BigEndian.PutUint16((*payload)[2:], 0)

	stream.SetDeadline(time.Now().Add(quicQueryTimeout))
	_, err = stream.Write(*payload)
	pool.ReleaseBuf(payload)
	if err != nil {
		stream.CancelRead(_DOQ_REQUEST_CANCELLED)
		stream.CancelWrite(_DOQ_REQUEST_CANCELLED)
		return nil, err
	}

	// RFC 9250 4.2
	//    The client MUST send the DNS query over the selected stream and MUST
	//    indicate through the STREAM FIN mechanism that no further data will
	//    be sent on that stream.
	//
	// Call Close() here will send the STREAM FIN. It won't close Read.
	stream.Close()

	type res struct {
		resp *[]byte
		err  error
	}
	rc := make(chan res, 1)
	go func() {
		r, err := dnsutils.ReadRawMsgFromTCP(stream)
		rc <- res{resp: r, err: err}
	}()

	select {
	case <-ctx.Done():
		stream.CancelRead(_DOQ_REQUEST_CANCELLED)
		return nil, context.Cause(ctx)
	case r := <-rc:
		resp := r.resp
		err := r.err
		if resp != nil {
			binary.BigEndian.PutUint16((*resp), orgQid)
		}
		stream.CancelRead(_DOQ_NO_ERROR)
		return resp, err
	}
}

func (ote *quicReservedExchanger) WithdrawReserved() {
	s := ote.stream
	s.CancelRead(_DOQ_REQUEST_CANCELLED)
	s.CancelWrite(_DOQ_REQUEST_CANCELLED)
}