package server

import (
	"context"
	"net/netip"

	"github.com/miekg/dns"
)

// Handler handles incoming request q and MUST ALWAYS return a response.
// Handler MUST handle dns errors by itself and return a proper error responses.
// e.g. Return a SERVFAIL if something goes wrong.
// If Handle() returns an error, caller considers that the error is associated
// with the downstream connection and will close the downstream connection
// immediately.
type Handler interface {
	Handle(ctx context.Context, q *dns.Msg, meta QueryMeta) (resp *dns.Msg, err error)
}

type QueryMeta struct {
	ClientAddr netip.Addr // Maybe invalid
	FromUDP    bool
}
