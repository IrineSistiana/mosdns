package fastforward

import (
	"context"
	"net"
	"time"

	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
)

type udpmeUpstream struct {
	addr    string
	trusted bool
}

func newUDPME(addr string, trusted bool) *udpmeUpstream {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "53")
	}
	return &udpmeUpstream{addr: addr, trusted: trusted}
}

func (u *udpmeUpstream) Address() string {
	return u.addr
}

func (u *udpmeUpstream) Trusted() bool {
	return u.trusted
}

func (u *udpmeUpstream) Exchange(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	ddl, ok := ctx.Deadline()
	if !ok {
		ddl = time.Now().Add(time.Second * 3)
	}

	if m.IsEdns0() != nil {
		return u.exchangeOPTM(m, ddl)
	}
	mc := m.Copy()
	mc.SetEdns0(512, false)
	r, err := u.exchangeOPTM(mc, ddl)
	if err != nil {
		return nil, err
	}
	dnsutils.RemoveEDNS0(r)
	return r, nil
}

func (u *udpmeUpstream) exchangeOPTM(m *dns.Msg, ddl time.Time) (*dns.Msg, error) {
	c, err := dns.Dial("udp", u.addr)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(ddl)
	if opt := m.IsEdns0(); opt != nil {
		c.UDPSize = opt.UDPSize()
	}

	if err := c.WriteMsg(m); err != nil {
		return nil, err
	}

	for {
		r, err := c.ReadMsg()
		if err != nil {
			return nil, err
		}
		if r.IsEdns0() == nil {
			continue
		}
		return r, nil
	}
}
