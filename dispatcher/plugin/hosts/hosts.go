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

package hosts

import (
	"bufio"
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"net"
	"os"
	"regexp"
	"strings"
)

const PluginType = "hosts"

func init() {
	handler.RegInitFunc(PluginType, Init)
}

var _ handler.Matcher = (*hostsContainer)(nil)

type Args struct {
	Hosts []string `yaml:"hosts"`
}

type hostsContainer struct {
	tag    string
	logger *logrus.Entry
	a      map[string][]net.IP
	aaaa   map[string][]net.IP
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	if len(args.Hosts) == 0 {
		return nil, errors.New("no hosts file is configured")
	}

	h := newHostsContainer(tag)
	for _, f := range args.Hosts {
		err := h.load(f)
		if err != nil {
			return nil, err
		}
	}

	return h, nil
}

func (h *hostsContainer) Tag() string {
	return h.tag
}

func (h *hostsContainer) Type() string {
	return PluginType
}

func (h *hostsContainer) Connect(ctx context.Context, qCtx *handler.Context, pipeCtx *handler.PipeContext) (err error) {
	err = h.connect(ctx, qCtx, pipeCtx)
	if err != nil {
		return handler.NewPluginError(h.tag, err)
	}
	return nil
}

func (h *hostsContainer) connect(ctx context.Context, qCtx *handler.Context, pipeCtx *handler.PipeContext) (err error) {
	if ok := h.matchAndSet(qCtx); ok {
		return nil
	}

	return pipeCtx.ExecNextPlugin(ctx, qCtx)
}

// Match matches domain in the hosts file and set its response.
// It never returns an err.
func (h *hostsContainer) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	return h.matchAndSet(qCtx), nil
}

func (h *hostsContainer) matchAndSet(qCtx *handler.Context) (matched bool) {
	if qCtx == nil || qCtx.Q == nil || len(qCtx.Q.Question) != 1 {
		return false
	}

	typ := qCtx.Q.Question[0].Qtype
	domain := qCtx.Q.Question[0].Name
	switch typ {
	case dns.TypeA:
		if ips, _ := h.a[domain]; len(ips) != 0 {
			r := new(dns.Msg)
			r.SetReply(qCtx.Q)
			for _, ip := range ips {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    3600,
					},
					A: ip,
				}
				r.Answer = append(r.Answer, rr)
			}
			qCtx.R = r
			return true
		}

	case dns.TypeAAAA:
		if ips, _ := h.aaaa[domain]; len(ips) != 0 {
			r := new(dns.Msg)
			r.SetReply(qCtx.Q)
			for _, ip := range ips {
				rr := &dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    3600,
					},
					AAAA: ip,
				}
				r.Answer = append(r.Answer, rr)
			}
			qCtx.R = r
			return true
		}
	}
	return false
}

func newHostsContainer(tag string) *hostsContainer {
	return &hostsContainer{
		tag:    tag,
		logger: mlog.NewPluginLogger(tag),
		a:      make(map[string][]net.IP),
		aaaa:   make(map[string][]net.IP),
	}
}

func (h *hostsContainer) load(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	r := regexp.MustCompile("\\S+")
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		t := strings.SplitN(scanner.Text(), "#", 2)[0] // remove strings after #

		e := r.FindAllString(t, -1)
		if len(e) == 0 {
			continue
		}

		if len(e) == 1 {
			h.logger.Warnf("invalid host record at line %d: %s", line, t)
			continue
		}

		host := dns.Fqdn(e[0])
		ips := e[1:]
		for _, ipStr := range ips {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				h.logger.Warnf("invalid ip addr %s at line %d: ", ipStr, line)
				continue
			}

			if ipv4 := ip.To4(); ipv4 != nil {
				h.a[host] = append(h.a[host], ipv4)
			} else if ipv6 := ip.To16(); ipv6 != nil {
				h.aaaa[host] = append(h.aaaa[host], ipv6)
			} else {
				h.logger.Warnf("invalid ip addr %s at line %d: ", ipStr, line)
				continue
			}
		}
	}
	return nil
}
