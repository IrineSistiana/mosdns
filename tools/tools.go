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

package tools

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func ProbServerTimeout(addr string) error {
	tryAddPort := func(addr string, defaultPort int) string {
		_, _, err := net.SplitHostPort(addr)
		if err != nil { // no port, add it.
			return net.JoinHostPort(addr, strconv.Itoa(defaultPort))
		}
		return addr
	}

	isTLS := false
	protocol, host := utils.SplitSchemeAndHost(addr)
	if len(protocol) == 0 || len(host) == 0 {
		return fmt.Errorf("invalid addr %s", addr)
	}

	switch protocol {
	case "tcp":
		isTLS = false
		host = tryAddPort(host, 53)
	case "dot", "tls":
		isTLS = true
		host = tryAddPort(host, 853)
	default:
		return fmt.Errorf("invalid protocol %s", protocol)
	}

	dialServer := func() (*dns.Conn, error) {
		var conn net.Conn
		var err error
		if isTLS {
			serverName, _, _ := net.SplitHostPort(host)
			tlsConfig := new(tls.Config)
			tlsConfig.InsecureSkipVerify = false
			tlsConfig.ServerName = serverName
			conn, err = net.Dial("tcp", host)
			if err != nil {
				return nil, err
			}
			tlsConn := tls.Client(conn, tlsConfig)
			tlsConn.SetDeadline(time.Now().Add(time.Second * 5))
			err = tlsConn.Handshake()
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("tls handshake failed: %v", err)
			}
			tlsConn.SetDeadline(time.Time{})
			conn = tlsConn
		} else {
			conn, err = net.Dial("tcp", host)
			if err != nil {
				return nil, err
			}
		}
		return &dns.Conn{Conn: conn}, nil
	}

	testBasicReuse := func() error {
		conn, err := dialServer()
		if err != nil {
			return err
		}
		defer conn.Close()
		for i := 0; i < 3; i++ {
			conn.SetDeadline(time.Now().Add(time.Second * 3))

			q := new(dns.Msg)
			q.SetQuestion("www.cloudflare.com.", dns.TypeA)
			q.Id = uint16(i)

			err = conn.WriteMsg(q)
			if err != nil {
				return fmt.Errorf("failed to write #%d probe msg: %v", i, err)
			}
			_, err = conn.ReadMsg()
			if err != nil {
				return fmt.Errorf("failed to read probe #%d msg response: %v", i, err)
			}
		}
		return nil
	}

	testOOOPipeline := func() (bool, error) {
		conn, err := dialServer()
		if err != nil {
			return false, err
		}
		defer conn.Close()
		domains := make([]string, 0)
		for i := 0; i < 4; i++ {
			b := make([]byte, 8)
			if _, err := rand.Read(b); err != nil {
				return false, err
			}
			domains = append(domains, fmt.Sprintf("www.%x.com.", b))
		}
		domains = append(domains, "www.cloudflare.com.")

		for i, d := range domains {
			conn.SetDeadline(time.Now().Add(time.Second * 10))

			q := new(dns.Msg)
			q.SetQuestion(d, dns.TypeA)
			q.Id = uint16(i)

			err = conn.WriteMsg(q)
			if err != nil {
				return false, fmt.Errorf("failed to write #%d probe msg: %v", i, err)
			}
		}

		oooPassed := false
		start := time.Now()
		for i := range domains {
			conn.SetDeadline(time.Now().Add(time.Second * 10))
			m, err := conn.ReadMsg()
			if err != nil {
				return false, fmt.Errorf("failed to read probe #%d msg response: %v", i, err)
			}

			mlog.S().Infof("#%d response received, latency: %d ms", m.Id, time.Since(start).Milliseconds())
			if m.Id != uint16(i) {
				oooPassed = true
				break
			}
		}

		return oooPassed, nil
	}

	waitIdleTimeout := func() error {
		conn, err := dialServer()
		if err != nil {
			return err
		}
		defer conn.Close()

		q := new(dns.Msg)
		q.SetQuestion("www.cloudflare.com.", dns.TypeA)

		err = conn.WriteMsg(q)
		if err != nil {
			return fmt.Errorf("failed to write probe msg: %v", err)
		}
		_, err = conn.ReadMsg()
		if err != nil {
			return fmt.Errorf("failed to read probe msg response: %v", err)
		}

		for {
			_, err := conn.ReadMsg()
			if err != nil {
				break
			}
		}
		return nil
	}

	mlog.S().Info("testing basic connection reuse")
	if err := testBasicReuse(); err != nil {
		mlog.S().Infof("× test failed: %v", err)
	} else {
		mlog.S().Info("√ basic connection reuse test passed")
	}

	mlog.S().Info("testing out-of-order pipeline")
	if ok, err := testOOOPipeline(); err != nil {
		mlog.S().Infof("× test failed: %v", err)
	} else {
		if ok {
			mlog.S().Info("√ out-of-order pipeline test passed")
		} else {
			mlog.S().Info("? test finished, no out-of-order responses")
		}
	}

	mlog.S().Info("testing idle timeout, awaiting server closing the connection, this may take a while")
	start := time.Now()
	if err := waitIdleTimeout(); err != nil {
		mlog.S().Infof("× test failed: %v", err)
	} else {
		mlog.S().Infof("√ connection closed by the server, its idle timeout is %.2f sec", time.Since(start).Seconds())
	}
	return nil
}

func BenchIPMatcher(f string) error {
	list := netlist.NewList()
	err := netlist.LoadFromFile(list, f)
	if err != nil {
		return err
	}
	list.Sort()

	ip := net.IPv4(8, 8, 8, 8).To4()

	start := time.Now()

	var n int = 1e4

	for i := 0; i < n; i++ {
		list.Match(ip)
	}
	timeCost := time.Since(start)

	mlog.S().Infof("%d ns/op", timeCost.Nanoseconds()/int64(n))
	return nil
}

func BenchDomainMatcher(f string) error {
	matcher := domain.NewMixMatcher()
	err := domain.LoadFromFile(matcher, f, nil)
	if err != nil {
		return err
	}
	start := time.Now()

	var n int = 1e4

	for i := 0; i < n; i++ {
		matcher.Match("www.goooooooooogle.com.")
	}
	timeCost := time.Since(start)

	mlog.S().Infof("%d ns/op", timeCost.Nanoseconds()/int64(n))
	return nil
}

func ConvertDomainDat(v string) error {
	s := strings.SplitN(v, ":", 2)
	datFileName := s[0]
	var wantTag string
	if len(s) == 2 {
		wantTag = strings.ToLower(s[1])
	}

	geoSiteList, err := v2data.LoadGeoSiteList(datFileName)
	if err != nil {
		return err
	}

	for _, geoSite := range geoSiteList.GetEntry() {
		tag := strings.ToLower(geoSite.GetCountryCode())

		if len(wantTag) != 0 && wantTag != tag {
			continue
		}

		file := fmt.Sprintf("%s_%s.txt", trimExt(datFileName), tag)
		mlog.S().Infof("saving %s domain to %s", tag, file)
		err := convertV2DomainToTextFile(geoSite.GetDomain(), file)
		if err != nil {
			return err
		}
	}
	return nil
}

func trimExt(f string) string {
	if i := strings.LastIndexByte(f, '.'); i == -1 {
		return f
	} else {
		return f[:i]
	}
}

func convertV2DomainToTextFile(domain []*v2data.Domain, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return convertV2DomainToText(domain, f)
}

func convertV2DomainToText(domain []*v2data.Domain, w io.Writer) error {
	for _, r := range domain {
		var prefix string
		switch r.Type {
		case v2data.Domain_Plain:
			prefix = "keyword:"
		case v2data.Domain_Regex:
			prefix = "regexp:"
		case v2data.Domain_Domain:
			prefix = ""
		case v2data.Domain_Full:
			prefix = "full:"
		default:
			return fmt.Errorf("invalid domain type %d", r.Type)
		}
		_, err := w.Write([]byte(prefix + r.Value + "\n"))
		if err != nil {
			return err
		}
	}
	return nil
}

func ConvertIPDat(v string) error {
	s := strings.SplitN(v, ":", 2)
	datFileName := s[0]
	var wantTag string
	if len(s) == 2 {
		wantTag = strings.ToLower(s[1])
	}

	geoIPList, err := v2data.LoadGeoIPListFromDAT(datFileName)
	if err != nil {
		return err
	}

	for _, ipList := range geoIPList.GetEntry() {
		tag := strings.ToLower(ipList.GetCountryCode())
		if len(wantTag) != 0 && wantTag != tag {
			continue
		}

		file := fmt.Sprintf("%s_%s.txt", trimExt(datFileName), tag)
		mlog.S().Infof("saving %s ip to %s", tag, file)
		err := convertV2CidrToTextFile(ipList.GetCidr(), file)
		if err != nil {
			return err
		}
	}

	return nil
}

func convertV2CidrToTextFile(cidr []*v2data.CIDR, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return convertV2CidrToText(cidr, f)
}

func convertV2CidrToText(cidr []*v2data.CIDR, w io.Writer) error {
	for _, record := range cidr {
		n := net.IPNet{
			IP: record.Ip,
		}
		switch len(record.Ip) {
		case 4:
			n.Mask = net.CIDRMask(int(record.Prefix), 32)
		case 16:
			n.Mask = net.CIDRMask(int(record.Prefix), 128)
		}
		_, err := w.Write([]byte(n.String() + "\n"))
		if err != nil {
			return err
		}
	}
	return nil
}
