package fastforward

import (
	"context"
	"net"
	"time"

	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/dnsutils"
	"github.com/miekg/dns"
)

type mustEDNS0Upstream struct {
	addr    string
	trusted bool
}

func newMEU(addr string, trusted bool) *mustEDNS0Upstream {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "53")
	}
	return &mustEDNS0Upstream{addr: addr, trusted: trusted}
}

func (u *mustEDNS0Upstream) Address() string {
	return u.addr
}

func (u *mustEDNS0Upstream) Trusted() bool {
	return u.trusted
}

func (u *mustEDNS0Upstream) Exchange(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
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

func (u *mustEDNS0Upstream) exchangeOPTM(m *dns.Msg, ddl time.Time) (*dns.Msg, error) {
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
