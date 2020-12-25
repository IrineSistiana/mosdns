//     Copyright (C) 2020, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package fastforward

import (
	"fmt"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/IrineSistiana/mosdns/dispatcher/plugin/executable/fast_forward/cpool"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

type fastUpstream struct {
	preferTCP bool
	addr      string
	logger    *logrus.Entry
	udpPool   *cpool.Pool
	tcpPool   *cpool.Pool
}

func newFastUpstream(addr string, preferTCP bool, logger *logrus.Entry) upstream.Upstream {
	return &fastUpstream{
		preferTCP: preferTCP,
		addr:      addr,
		udpPool:   cpool.New(0xffff, time.Second*30, time.Second*5, logger),
		tcpPool:   cpool.New(0xffff, tcpIdleTimeout, time.Second*2, logger),
	}
}

func (u *fastUpstream) Address() string {
	if u.preferTCP {
		return "tcp://" + u.addr
	}
	return "udp://" + u.addr
}

func (u *fastUpstream) Exchange(q *dns.Msg) (r *dns.Msg, err error) {
	if u.preferTCP {
		return u.exchangeTCP(q)
	}

	r, err = u.exchangeUDP(q)
	if err != nil {
		return
	}

	if r != nil && r.Truncated {
		return u.exchangeTCP(q)
	}
	return
}

func (u *fastUpstream) exchangeTCP(q *dns.Msg) (r *dns.Msg, err error) {
	c := u.tcpPool.Get()
	start := time.Now()

exchange:
	isNewConn := false
	if c == nil {
		dialer := net.Dialer{Timeout: dialTimeout}
		c, err = dialer.Dial("tcp", u.addr)
		if err != nil {
			return nil, fmt.Errorf("failed to dial new tcp conntion: %w", err)
		}
		isNewConn = true
	}

	r, err = u.exchangeViaTCPConn(q, c)
	if err != nil {
		c.Close()
		if isNewConn || time.Now().Sub(start) > time.Millisecond*100 {
			return nil, err
		} else {
			// There has a race condition between client and server.
			// If reused connection returned an err very quickly,
			// Dail a new connection and try again.
			c = nil
			goto exchange
		}
	}

	u.tcpPool.Put(c)
	return r, nil
}

func (u *fastUpstream) exchangeViaTCPConn(q *dns.Msg, c net.Conn) (r *dns.Msg, err error) {
	c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err = utils.WriteMsgToTCP(c, q)
	if err != nil {
		return nil, err
	}
	c.SetReadDeadline(time.Now().Add(generalReadTimeout))
	r, _, err = utils.ReadMsgFromTCP(c)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (u *fastUpstream) exchangeUDP(q *dns.Msg) (r *dns.Msg, err error) {
	c := u.udpPool.Get()
	isNewConn := false
	if c == nil {
		dialer := net.Dialer{Timeout: dialTimeout}
		c, err = dialer.Dial("udp", u.addr)
		if err != nil {
			return nil, fmt.Errorf("failed to dial new udp conntion: %w", err)
		}
		isNewConn = true
	}

	r, err = u.exchangeViaUDPConn(q, c, isNewConn)
	if err != nil {
		c.Close()
		return nil, err
	}

	u.udpPool.Put(c)
	return r, nil
}

func (u *fastUpstream) exchangeViaUDPConn(q *dns.Msg, c net.Conn, isNewConn bool) (r *dns.Msg, err error) {
	c.SetWriteDeadline(time.Now().Add(generalWriteTimeout))
	_, err = utils.WriteMsgToUDP(c, q)
	if err != nil { // write err typically is a fatal err
		return nil, err
	}
	c.SetReadDeadline(time.Now().Add(generalReadTimeout))

	if isNewConn {
		r, _, err = utils.ReadMsgFromUDP(c, utils.IPv4UdpMaxPayload)
		return r, err
	} else {

		// Reused udp sockets might have dirty data in the read buffer,
		// say, multi-package dns pollution.
		// This is a simple workaround.
		for {
			r, _, err = utils.ReadMsgFromUDP(c, utils.IPv4UdpMaxPayload)
			if err != nil {
				return nil, err
			}

			// id mismatch, ignore it and read again.
			if r.Id != q.Id {
				continue
			}
			return r, nil
		}
	}
}
