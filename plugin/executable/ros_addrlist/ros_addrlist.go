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

package ros_addrlist

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/netip"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const PluginType = "ros_addrlist"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

type Args struct {
	AddrList string `yaml:"addrlist"`
	Server   string `yaml:"server"`
	User     string `yaml:"user"`
	Passwd   string `yaml:"passwd"`
	Mask4    int    `yaml:"mask4"` // default 24
	Mask6    int    `yaml:"mask6"` // default 32
}

type rosAddrlistPlugin struct {
	args   *Args
	client *http.Client
}

func newRosAddrlistPlugin(args *Args) (*rosAddrlistPlugin, error) {
	if args.Mask4 == 0 {
		args.Mask4 = 24
	}
	if args.Mask6 == 0 {
		args.Mask6 = 32
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout: 30 * time.Second,
		MaxIdleConns:    10,
	}
	client := &http.Client{
		Timeout:   time.Second * 2,
		Transport: tr,
	}

	return &rosAddrlistPlugin{
		args:   args,
		client: client,
	}, nil
}

func (p *rosAddrlistPlugin) Exec(ctx context.Context, qCtx *query_context.Context) error {
	r := qCtx.R()
	if r != nil {
		if err := p.addIP(r); err != nil {
			fmt.Printf("ros_addrlist addip failed but ignored: %w", err)
		}
	}
	return nil
}

func (p *rosAddrlistPlugin) addIPViaHTTPRequest(ip *net.IP, v6 bool, from string) error {
	// request to add ips via http request routeros RESTFul API
	t := "ip"
	if v6 {
		t = "ipv6"
	}
	routerURL := p.args.Server + "/rest/" + t + "/firewall/address-list/add"
	payload := map[string]interface{}{
		"address": ip.String(),
		"list":    p.args.AddrList,
		"comment": "[mosdns] domain: " + from,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal json data: %w", err)
	}

	req, err := http.NewRequest("POST", routerURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(p.args.User, p.args.Passwd)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (p *rosAddrlistPlugin) addIP(r *dns.Msg) error {
	for i := range r.Answer {
		switch rr := r.Answer[i].(type) {
		case *dns.A:
			if len(p.args.AddrList) == 0 {
				continue
			}
			_, ok := netip.AddrFromSlice(rr.A.To4())
			if !ok {
				return fmt.Errorf("invalid A record with ip: %s", rr.A)
			}
			if err := p.addIPViaHTTPRequest(&rr.A, false, r.Question[0].Name); err != nil {
				fmt.Printf("failed to add ip: %s, %v\n", rr.A, err)
				return err
			}

		case *dns.AAAA:
			if len(p.args.AddrList) == 0 {
				continue
			}
			_, ok := netip.AddrFromSlice(rr.AAAA.To16())
			if !ok {
				return fmt.Errorf("invalid AAAA record with ip: %s", rr.AAAA)
			}
			if err := p.addIPViaHTTPRequest(&rr.AAAA, true, r.Question[0].Name); err != nil {
				fmt.Printf("failed to add ip: %s, %v\n", rr.AAAA, err)
                                return err
			}
		default:
			continue
		}
	}

	return nil
}

func (p *rosAddrlistPlugin) Close() error {
	return nil
}

// QuickSetup format: [set_name,{inet|inet6},mask] *2
// e.g. "http://192.168.111.1:8080,admin,password,gfwlist,inet,24"
func QuickSetup(_ sequence.BQ, s string) (any, error) {
	fs := strings.Fields(s)
	if len(fs) > 6 {
		return nil, fmt.Errorf("expect no more than 6 fields, got %d", len(fs))
	}

	args := new(Args)
	for _, argsStr := range fs {
		ss := strings.Split(argsStr, ",")
		if len(ss) != 6 {
			return nil, fmt.Errorf("invalid args, expect 6 fields, got %d", len(ss))
		}

		m, err := strconv.Atoi(ss[5])
		if err != nil {
			return nil, fmt.Errorf("invalid mask, %w", err)
		}
		args.Mask4 = m

		args.Server = ss[0]
		args.User = ss[1]
		args.Passwd = ss[2]
		args.AddrList = ss[3]
		switch ss[4] {
		case "inet":
			args.Mask4 = m
		case "inet6":
			args.Mask6 = m
		default:
			return nil, fmt.Errorf("invalid set family, %s", ss[0])
		}
	}
	return newRosAddrlistPlugin(args)
}
