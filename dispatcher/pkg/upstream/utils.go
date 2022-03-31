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
	"github.com/miekg/dns"
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
