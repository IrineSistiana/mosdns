/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const (
	minimumUpdateInterval = time.Minute * 5
	retryInterval         = time.Second * 2
	queryTimeout          = time.Second * 5
)

var (
	errNoAddrInResp = errors.New("resp does not have ip address")
)

func New(
	host string,
	port uint16,
	bootstrapServer netip.AddrPort,
	bootstrapVer int, // 0,4,6
	logger *zap.Logger, // not nil
) (*Bootstrap, error) {
	dp := new(Bootstrap)
	dp.fqdn = dns.Fqdn(host)
	dp.port = port
	if !bootstrapServer.IsValid() {
		return nil, errors.New("invalid bootstrap server address")
	}
	dp.bootstrap = net.UDPAddrFromAddrPort(bootstrapServer)
	qt, ok := bootstrapVer2Qt(bootstrapVer)
	if !ok {
		return nil, fmt.Errorf("invalid bootstrap version %d", bootstrapVer)
	}
	dp.qt = qt
	dp.logger = logger

	dp.readyNotify = make(chan struct{})
	return dp, nil
}

type Bootstrap struct {
	fqdn      string
	port      uint16
	bootstrap *net.UDPAddr
	qt        uint16      // dns.TypeA or dns.TypeAAAA
	logger    *zap.Logger // not nil

	updating   atomic.Bool
	nextUpdate time.Time

	readyNotify chan struct{}
	m           sync.Mutex
	ready       bool
	addrStr     string
}

func (sp *Bootstrap) GetAddrPortStr(ctx context.Context) (string, error) {
	sp.tryUpdate()

	select {
	case <-ctx.Done():
		return "", context.Cause(ctx)
	case <-sp.readyNotify:
	}

	sp.m.Lock()
	addr := sp.addrStr
	sp.m.Unlock()
	return addr, nil
}

func (sp *Bootstrap) tryUpdate() {
	if sp.updating.CompareAndSwap(false, true) {
		if time.Now().After(sp.nextUpdate) {
			go func() {
				defer sp.updating.Store(false)
				ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
				defer cancel()
				start := time.Now()
				addr, ttl, err := sp.updateAddr(ctx)
				if err != nil {
					sp.logger.Check(zap.WarnLevel, "failed to update bootstrap addr").Write(
						zap.String("fqdn", sp.fqdn),
						zap.Error(err),
					)
					sp.nextUpdate = time.Now().Add(retryInterval)
				} else {
					updateInterval := time.Second * time.Duration(ttl)
					if updateInterval < minimumUpdateInterval {
						updateInterval = minimumUpdateInterval
					}
					sp.logger.Check(zap.DebugLevel, "bootstrap addr updated").Write(
						zap.String("fqdn", sp.fqdn),
						zap.Stringer("addr", addr),
						zap.Duration("ttl", updateInterval),
						zap.Duration("elapse", time.Since(start)),
					)
					sp.nextUpdate = time.Now().Add(updateInterval)
				}
			}()
		} else {
			sp.updating.Store(false)
		}
	}
}

func (sp *Bootstrap) updateAddr(ctx context.Context) (netip.Addr, uint32, error) {
	addr, ttl, err := sp.resolve(ctx, sp.qt)
	if err != nil {
		return netip.Addr{}, 0, err
	}

	addrPort := netip.AddrPortFrom(addr, sp.port).String()
	sp.m.Lock()
	sp.addrStr = addrPort
	if !sp.ready {
		sp.ready = true
		close(sp.readyNotify)
	}
	sp.m.Unlock()
	return addr, ttl, nil
}

func (sp *Bootstrap) resolve(ctx context.Context, qt uint16) (netip.Addr, uint32, error) {
	const edns0UdpSize = 1200

	q := new(dns.Msg)
	q.SetQuestion(sp.fqdn, qt)
	q.SetEdns0(edns0UdpSize, false)

	c, err := net.DialUDP("udp", nil, sp.bootstrap)
	if err != nil {
		return netip.Addr{}, 0, err
	}
	defer c.Close()

	writeErrC := make(chan error, 1)
	type res struct {
		resp *dns.Msg
		err  error
	}
	readResC := make(chan res, 1)

	cancelWrite := make(chan struct{})
	defer close(cancelWrite)
	go func() {
		if _, err := dnsutils.WriteMsgToUDP(c, q); err != nil {
			writeErrC <- err
			return
		}

		retryTicker := time.NewTicker(time.Second)
		defer retryTicker.Stop()
		for {
			select {
			case <-cancelWrite:
				return
			case <-retryTicker.C:
				if _, err := dnsutils.WriteMsgToUDP(c, q); err != nil {
					writeErrC <- err
					return
				}
			}
		}
	}()

	go func() {
		m, _, err := dnsutils.ReadMsgFromUDP(c, edns0UdpSize)
		readResC <- res{resp: m, err: err}
	}()

	select {
	case <-ctx.Done():
		return netip.Addr{}, 0, context.Cause(ctx)
	case err := <-writeErrC:
		return netip.Addr{}, 0, fmt.Errorf("failed to write query, %w", err)
	case r := <-readResC:
		resp := r.resp
		err := r.err
		if err != nil {
			return netip.Addr{}, 0, fmt.Errorf("failed to read resp, %w", err)
		}

		for _, v := range resp.Answer {
			var ip net.IP
			var ttl uint32
			switch rr := v.(type) {
			case *dns.A:
				ip = rr.A
				ttl = rr.Hdr.Ttl
			case *dns.AAAA:
				ip = rr.AAAA
				ttl = rr.Hdr.Ttl
			default:
				continue
			}
			addr, ok := netip.AddrFromSlice(ip)
			if ok {
				return addr, ttl, nil
			}
		}

		// No ip addr in resp.
		return netip.Addr{}, 0, errNoAddrInResp
	}
}

func bootstrapVer2Qt(ver int) (uint16, bool) {
	switch ver {
	case 0, 4:
		return dns.TypeA, true
	case 6:
		return dns.TypeAAAA, true
	default:
		return 0, false
	}
}
