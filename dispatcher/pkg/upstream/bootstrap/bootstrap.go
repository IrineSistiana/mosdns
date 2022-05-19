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

package bootstrap

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/upstream"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	bootstrapTimeout       = time.Second * 5
	bootstrapRetryInterval = time.Second * 2
)

type BootstrapMode int

// BootstrapMode
const (
	BootstrapModeInvalid BootstrapMode = iota
	BootstrapModeV4
	BootstrapModeV6
)

const (
	DefaultMinimumUpdateInterval = time.Second * 600  // 10 min
	DefaultMaximumUpdateInterval = time.Second * 3600 // 1 hour
)

var (
	nopLogger = zap.NewNop()
)

type Bootstrap struct {
	// Fqdn is the fully qualified domain name that needs to be resolved.
	Fqdn string

	// Upstream for this Bootstrap to send queries to.
	Upstream upstream.Upstream

	// Mode specifies whether this Bootstrap works on IPv4 or IPv6.
	// A zero value Mode is invalid.
	Mode BootstrapMode

	// MinimumUpdateInterval specifies the minimum update interval.
	// Default is DefaultMinimumUpdateInterval.
	MinimumUpdateInterval time.Duration

	// MaximumUpdateInterval specifies the maximum update interval.
	// Default is DefaultMaximumUpdateInterval.
	MaximumUpdateInterval time.Duration

	Logger *zap.Logger

	m          sync.Mutex
	booted     chan struct{}
	ipAddr     string
	expireTime time.Time

	updating       uint32 // atomic
	lastUpdateTime time.Time
}

func (b *Bootstrap) logger() *zap.Logger {
	if b.Logger != nil {
		return b.Logger
	}
	return nopLogger
}

func (b *Bootstrap) minimumUpdateInterval() time.Duration {
	if b.MinimumUpdateInterval > 0 {
		return b.MinimumUpdateInterval
	}
	return DefaultMinimumUpdateInterval
}

func (b *Bootstrap) maximumUpdateInterval() time.Duration {
	if b.MaximumUpdateInterval > 0 {
		return b.MaximumUpdateInterval
	}
	return DefaultMaximumUpdateInterval
}

func (b *Bootstrap) GetAddr(ctx context.Context) (string, error) {
	now := time.Now()

	b.m.Lock()
	if b.booted == nil {
		b.booted = make(chan struct{})
	}

	if b.expireTime.Before(now) {
		b.tryBackgroundUpdate()
	}

	if ip := b.ipAddr; len(ip) != 0 {
		b.m.Unlock()
		return ip, nil
	}
	b.m.Unlock()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-b.booted:
		b.m.Lock()
		ip := b.ipAddr
		b.m.Unlock()
		return ip, nil
	}
}

func (b *Bootstrap) tryBackgroundUpdate() {
	if atomic.CompareAndSwapUint32(&b.updating, 0, 1) {
		defer atomic.StoreUint32(&b.updating, 0)
	} else {
		return
	}

	if b.lastUpdateTime.Add(bootstrapRetryInterval).After(time.Now()) {
		return
	}

	go func() {
		ip, ttl, err := b.update()
		if err != nil {
			b.logger().Warn("failed to update bootstrap", zap.Error(err))
			return
		}

		b.logger().Debug("bootstrap address updated", zap.Stringer("new_addr", ip), zap.Duration("ttl", ttl))
		expireTime := time.Now().Add(ttl)

		b.m.Lock()
		b.ipAddr = ip.String()
		b.expireTime = expireTime
		if b.booted != nil && !utils.ClosedChan(b.booted) {
			close(b.booted)
		}
		b.m.Unlock()
	}()
}

func (b *Bootstrap) restrictTtl(ttl time.Duration) time.Duration {
	if minTtl := b.minimumUpdateInterval(); ttl < minTtl {
		ttl = minTtl
	}
	if maxTtl := b.maximumUpdateInterval(); ttl > maxTtl {
		ttl = maxTtl
	}
	return ttl
}

func (b *Bootstrap) update() (net.IP, time.Duration, error) {
	m := new(dns.Msg)
	var qType uint16
	switch b.Mode {
	case BootstrapModeV4:
		qType = dns.TypeA
	case BootstrapModeV6:
		qType = dns.TypeAAAA
	default:
		panic(fmt.Sprintf("invalid bootstrap mode %d", b.Mode))
	}
	m.SetQuestion(b.Fqdn, qType)

	ctx, cancel := context.WithTimeout(context.Background(), bootstrapTimeout)
	defer cancel()
	r, err := b.Upstream.ExchangeContext(ctx, m)
	if err != nil {
		return nil, 0, fmt.Errorf("upstream failed, %w", err)
	}

	var (
		ip  net.IP
		ttl uint32
	)

	for _, rr := range r.Answer {
		switch rr := rr.(type) {
		case *dns.A:
			ip = rr.A
			ttl = rr.Hdr.Ttl
		case *dns.AAAA:
			ip = rr.AAAA
			ttl = rr.Hdr.Ttl
		}
	}

	if ip == nil {
		return nil, 0, fmt.Errorf("response does not have valid ip, [%s]", r)
	}
	return ip, b.restrictTtl(time.Duration(ttl) * time.Second), nil
}
