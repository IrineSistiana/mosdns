//     Copyright (C) 2020-2021, IrineSistiana
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

package upstream

import (
	"context"
	"fmt"
	"github.com/miekg/dns"
	"golang.org/x/net/proxy"
	"net"
	"time"
)

// getContextDeadline tries to get the deadline of ctx or return a default
// deadline.
func getContextDeadline(ctx context.Context, defTimeout time.Duration) time.Time {
	ddl, ok := ctx.Deadline()
	if ok {
		return ddl
	}
	return time.Now().Add(defTimeout)
}

func shadowCopy(m *dns.Msg) *dns.Msg {
	nm := new(dns.Msg)
	*nm = *m
	return nm
}

func chanClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

func dialTCP(ctx context.Context, addr, socks5 string, mark int) (net.Conn, error) {
	if len(socks5) > 0 {
		socks5Dialer, err := proxy.SOCKS5("tcp", socks5, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to init socks5 dialer: %w", err)
		}
		return socks5Dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", addr)
	}

	d := net.Dialer{Control: getSetMarkFunc(mark)}
	return d.DialContext(ctx, "tcp", addr)
}
