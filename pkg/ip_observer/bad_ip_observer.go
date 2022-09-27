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

package ip_observer

import (
	"bytes"
	"github.com/IrineSistiana/mosdns/v4/pkg/concurrent_limiter"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"go.uber.org/zap"
	"net/http"
	"net/netip"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type BadIPObserverOpts struct {
	HPLimiterOpts concurrent_limiter.HPLimiterOpts

	// TTL specifies the duration that a bad ip will stay in the
	// BadIPObserver if no error is reported. Optional. Default is 10min.
	TTL time.Duration

	// OnUpdateCallBack specifies a local cmd or url that will be called if
	// the bad ip list is updated. Optional.
	// If it starts with "http://" or "https://", BadIPObserverOpts will post
	// a list of bad ip/cidr, split by "/n", to this url.
	// Otherwise, BadIPObserverOpts trades this as a local cmd and will execute
	// it. User should fetch the list of bad ips themselves. OnUpdateCallBack
	// implements the http.Handler interface.
	OnUpdateCallBack string

	CleanerInterval time.Duration // Optional. Default is 10s.
	Logger          *zap.Logger   // Optional. Default is a noop logger.
}

func (opts *BadIPObserverOpts) init() error {
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}
	utils.SetDefaultNum(&opts.TTL, time.Minute*10)
	utils.SetDefaultNum(&opts.CleanerInterval, time.Second*10)
	return nil
}

var _ IPObserver = (*BadIPObserver)(nil)

type BadIPObserver struct {
	opts BadIPObserverOpts

	errLimiter *concurrent_limiter.HPClientLimiter

	m     sync.Mutex               // Protect badIP and lastUpdateTime
	badIP map[netip.Addr]time.Time // map[<masked_bad_addr>]<last_seen_time>

	callbackDeferMutex sync.Mutex
	callbackDeferTimer *time.Timer

	closeOnce   sync.Once
	closeNotify chan struct{}
}

func NewBadIPObserver(opts BadIPObserverOpts) (*BadIPObserver, error) {
	if err := opts.init(); err != nil {
		return nil, err
	}
	hpLimiter, err := concurrent_limiter.NewHPClientLimiter(opts.HPLimiterOpts)
	if err != nil {
		return nil, err
	}

	w := &BadIPObserver{
		opts:        opts,
		errLimiter:  hpLimiter,
		badIP:       make(map[netip.Addr]time.Time),
		closeNotify: make(chan struct{}),
	}
	go w.cleanerLoop()
	return w, nil
}

// Observe notifies the BadIPObserver that this addr has one error event.
func (o *BadIPObserver) Observe(addr netip.Addr) {
	if tooManyErr := o.errLimiter.AcquireToken(addr); tooManyErr {
		prefix := o.errLimiter.ApplyMask(addr)
		maskedAddr := prefix.Addr()
		o.m.Lock()
		_, existed := o.badIP[maskedAddr]
		o.badIP[maskedAddr] = time.Now()
		o.m.Unlock()
		if !existed {
			o.notifyUpdateCallback()
			o.opts.Logger.Warn("new bad ip", zap.Stringer("ip", prefix))
		}
	}
}

// Close stops callback goroutine. It always returns a nil err.
func (o *BadIPObserver) Close() error {
	o.closeOnce.Do(func() {
		o.errLimiter.Close()
		close(o.closeNotify)
	})
	return nil
}

func (o *BadIPObserver) cleanerLoop() {
	ticker := time.NewTicker(o.opts.CleanerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			updated := o.runCleaner()
			if updated {
				o.notifyUpdateCallback()
			}
		case <-o.closeNotify:
			return
		}
	}
}

func (o *BadIPObserver) runCleaner() (updated bool) {
	now := time.Now()
	o.m.Lock()
	defer o.m.Unlock()
	for addr, lastSeen := range o.badIP {
		if now.Sub(lastSeen) > o.opts.TTL {
			delete(o.badIP, addr)
			updated = true
		}
	}
	return updated
}

func (o *BadIPObserver) notifyUpdateCallback() {
	callbackCmd := o.opts.OnUpdateCallBack
	if len(callbackCmd) == 0 {
		return
	}

	o.callbackDeferMutex.Lock()
	defer o.callbackDeferMutex.Unlock()

	if o.callbackDeferTimer == nil {
		o.callbackDeferTimer = time.AfterFunc(time.Second*2, func() { // Callback only be called one time per 2 sec.
			o.callbackDeferMutex.Lock()
			o.callbackDeferTimer.Stop()
			o.callbackDeferTimer = nil
			o.callbackDeferMutex.Unlock()

			o.runUpdateCallback()
		})
	}
}

func (o *BadIPObserver) runUpdateCallback() {
	callbackCmd := o.opts.OnUpdateCallBack
	o.opts.Logger.Info("executing bad ip update callback", zap.String("cmd", callbackCmd))
	if strings.HasPrefix(callbackCmd, "http://") || strings.HasPrefix(callbackCmd, "https://") {
		resp, err := http.Post(callbackCmd, "", o.makeList())
		if err != nil {
			o.opts.Logger.Error("http callback err", zap.Error(err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			o.opts.Logger.Error("http callback bad status code", zap.Int("status", resp.StatusCode))
		}
		return
	}

	cmd := exec.Command(callbackCmd)
	if err := cmd.Run(); err != nil {
		o.opts.Logger.Error("callback cmd err", zap.Error(err))
	}
}

type kv struct {
	a netip.Addr
	t time.Time
}

func (o *BadIPObserver) dumpMapAddrData() []kv {
	o.m.Lock()
	defer o.m.Unlock()
	s := make([]kv, 0, len(o.badIP))
	for addr, t := range o.badIP {
		s = append(s, kv{a: addr, t: t})
	}
	return s
}

func (o *BadIPObserver) makeList() *bytes.Buffer {
	s := new(bytes.Buffer)
	maskV4 := o.opts.HPLimiterOpts.IPv4Mask
	maskV6 := o.opts.HPLimiterOpts.IPv4Mask
	d := o.dumpMapAddrData()
	for _, e := range d {
		addr := e.a
		if addr.Is4() || addr.Is4In6() {
			prefix := netip.PrefixFrom(addr.Unmap(), maskV4).Masked()
			s.WriteString(prefix.String())

		} else {
			prefix := netip.PrefixFrom(addr, maskV6).Masked()
			s.WriteString(prefix.String())
		}
		s.WriteRune('\n')
	}
	return s
}

func (o *BadIPObserver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Write(o.makeList().Bytes())
}
