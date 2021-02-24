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
	"crypto/tls"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/v2data"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

func ProbServerTimeout(addr string) error {
	isTLS := false
	protocol, host := utils.SplitSchemeAndHost(addr)
	if len(protocol) == 0 || len(host) == 0 {
		return fmt.Errorf("invalid addr %s", addr)
	}

	switch protocol {
	case "tcp":
		isTLS = false
	case "dot":
		isTLS = true
	default:
		return fmt.Errorf("invalid protocol %s", protocol)
	}

	q := new(dns.Msg)
	q.SetQuestion("www.google.com.", dns.TypeA)

	var conn net.Conn
	var err error

	mlog.S().Infof("connecting to %s", addr)
	if isTLS {
		tlsConfig := new(tls.Config)
		tlsConfig.InsecureSkipVerify = true
		tlsConn, err := tls.Dial("tcp", host, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to dail tls connection: %v", err)
		}
		tlsConn.SetDeadline(time.Now().Add(time.Second * 5))
		mlog.S().Info("connected, start TLS handshaking")
		err = tlsConn.Handshake()
		if err != nil {
			return fmt.Errorf("tls handshake failed: %v", err)
		}
		mlog.S().Info("TLS handshake completed")
		conn = tlsConn
	} else {
		conn, err = net.Dial("tcp", host)
		if err != nil {
			return fmt.Errorf("can not connect to server: %v", err)
		}
	}
	defer conn.Close()
	mlog.S().Info("server connected")
	mlog.S().Info("starting rfc 7766 tcp connection reuse test")

	for i := 0; i < 2; i++ {
		conn.SetDeadline(time.Now().Add(time.Second * 3))
		dc := dns.Conn{Conn: conn}
		err = dc.WriteMsg(q)
		if err != nil {
			return fmt.Errorf("test failed: failed to write probe msg: %v", err)
		}
		_, err = dc.ReadMsg()
		if err != nil {
			return fmt.Errorf("test failed: failed to read probe msg response: %v", err)
		}
	}

	mlog.S().Info("test passed")
	mlog.S().Info("testing server idle timeout. this may take a while...")
	mlog.S().Info("if you think its long enough, to cancel the test, press Ctrl + C")
	conn.SetDeadline(time.Now().Add(time.Minute * 60))

	start := time.Now()
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err == nil {
		return fmt.Errorf("recieved unexpected data from peer: %v", buf[:n])
	}

	mlog.S().Infof("server idle timeout: %.2f sec", time.Since(start).Seconds())
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

	var n int = 1e6

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

	var n int = 1e6

	for i := 0; i < n; i++ {
		matcher.Match("com.")
	}
	timeCost := time.Since(start)

	mlog.S().Infof("%d ns/op", timeCost.Nanoseconds()/int64(n))
	return nil
}

func ConvertDomainDat(v string) error {
	s := strings.SplitN(v, ":", 2)
	datFile := s[0]
	var wantTag string
	if len(s) == 2 {
		wantTag = s[1]
	}

	geoSiteList, err := domain.LoadGeoSiteList(datFile)
	if err != nil {
		return err
	}

	wantTag = strings.ToLower(wantTag)
	for _, geoSite := range geoSiteList.GetEntry() {
		tag := strings.ToLower(geoSite.GetCountryCode())

		if len(wantTag) != 0 && wantTag != tag {
			continue
		}

		file := fmt.Sprintf("%s_domain.list", tag)
		mlog.S().Infof("saving %s domain to %s", tag, file)
		err := convertV2DomainToTextFile(geoSite.GetDomain(), file)
		if err != nil {
			return err
		}
	}
	return nil
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
	file := s[0]
	var wantTag string
	if len(s) == 2 {
		wantTag = s[1]
	}

	geoIPList, err := netlist.LoadGeoIPListFromDAT(file)
	if err != nil {
		return err
	}

	for _, ipList := range geoIPList.GetEntry() {
		tag := strings.ToLower(ipList.GetCountryCode())
		if len(wantTag) != 0 && wantTag != tag {
			continue
		}

		file := fmt.Sprintf("%s_ip.list", tag)
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
