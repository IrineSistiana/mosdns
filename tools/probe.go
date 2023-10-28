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

package tools

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/miekg/dns"
	"github.com/spf13/cobra"
)

func newIdleTimeoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "idle-timeout {tcp|tls}://server_addr[:port]",
		Args:  cobra.ExactArgs(1),
		Short: "Probe server's idle timeout.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := ProbServerTimeout(args[0]); err != nil {
				mlog.S().Fatal(err)
			}
		},
		DisableFlagsInUseLine: true,
	}
}

func newConnReuseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "conn-reuse {tcp|tls}://server_addr[:port]",
		Args:  cobra.ExactArgs(1),
		Short: "Check whether this server supports RFC 1035 connection reuse.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := ProbServerConnectionReuse(args[0]); err != nil {
				mlog.S().Fatal(err)
			}
		},
		DisableFlagsInUseLine: true,
	}
}

func newPipelineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pipeline {tcp|tls}://server_addr[:port]",
		Args:  cobra.ExactArgs(1),
		Short: "Check whether this server supports RFC 7766 query pipelining.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := ProbServerPipeline(args[0]); err != nil {
				mlog.S().Fatal(err)
			}
		},
		DisableFlagsInUseLine: true,
	}
}

func getConn(addr string) (net.Conn, error) {
	tryAddPort := func(addr string, defaultPort int) string {
		_, _, err := net.SplitHostPort(addr)
		if err != nil { // no port, add it.
			return net.JoinHostPort(addr, strconv.Itoa(defaultPort))
		}
		return addr
	}

	protocol, host := utils.SplitSchemeAndHost(addr)
	if len(protocol) == 0 || len(host) == 0 {
		return nil, fmt.Errorf("invalid addr %s", addr)
	}

	switch protocol {
	case "tcp":
		host = tryAddPort(host, 53)
		return net.Dial("tcp", host)
	case "tls":
		host = tryAddPort(host, 853)
		serverName, _, _ := net.SplitHostPort(host)
		tlsConfig := new(tls.Config)
		tlsConfig.InsecureSkipVerify = false
		tlsConfig.ServerName = serverName
		conn, err := net.Dial("tcp", host)
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
		return tlsConn, nil
	default:
		return nil, fmt.Errorf("invalid protocol %s", protocol)
	}
}

func ProbServerConnectionReuse(addr string) error {
	c, err := getConn(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	conn := dns.Conn{Conn: c}
	for i := 0; i < 3; i++ {
		conn.SetDeadline(time.Now().Add(time.Second * 3))

		q := new(dns.Msg)
		q.SetQuestion("www.cloudflare.com.", dns.TypeA)
		q.Id = uint16(i)

		mlog.S().Infof("sending msg #%d", i)
		err = conn.WriteMsg(q)
		if err != nil {
			return fmt.Errorf("failed to write #%d probe msg: %v", i, err)
		}
		_, err = conn.ReadMsg()
		if err != nil {
			return fmt.Errorf("failed to read #%d probe msg response: %v", i, err)
		}
		mlog.S().Infof("recevied response #%d", i)
	}

	mlog.S().Infof("server %s supports RFC 1035 connection reuse", addr)
	return nil
}

func ProbServerPipeline(addr string) error {
	c, err := getConn(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	conn := dns.Conn{Conn: c}
	if err != nil {
		return err
	}
	defer conn.Close()

	domains := make([]string, 0)
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	domains = append(domains, fmt.Sprintf("%x.com.", b))
	domains = append(domains, ".")

	for i, d := range domains {
		conn.SetDeadline(time.Now().Add(time.Second * 3))

		q := new(dns.Msg)
		q.SetQuestion(d, dns.TypeNS)
		q.Id = uint16(i)

		err = conn.WriteMsg(q)
		if err != nil {
			return fmt.Errorf("failed to write #%d probe msg: %v", i, err)
		}
	}

	oooPassed := false
	start := time.Now()
	for i := range domains {
		conn.SetDeadline(time.Now().Add(time.Second * 10))
		m, err := conn.ReadMsg()
		if err != nil {
			return fmt.Errorf("failed to read #%d probe msg response: %v", i, err)
		}

		mlog.S().Infof("#%d response received, latency: %d ms", m.Id, time.Since(start).Milliseconds())
		if m.Id != uint16(i) {
			oooPassed = true
		}
	}

	if oooPassed {
		mlog.S().Info("server supports RFC7766 query pipelining")
	} else {
		mlog.S().Info("no out-of-order response received in this test, server MAY NOT support RFC7766 query pipelining")
	}
	return nil
}

func ProbServerTimeout(addr string) error {
	c, err := getConn(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	conn := dns.Conn{Conn: c}
	q := new(dns.Msg)
	q.SetQuestion("www.cloudflare.com.", dns.TypeA)
	err = conn.WriteMsg(q)
	if err != nil {
		return fmt.Errorf("failed to write probe msg: %v", err)
	}

	mlog.S().Info("testing server idle timeout, awaiting server closing the connection, this may take a while")
	start := time.Now()
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
	mlog.S().Infof("connection closed by peer, it's idle timeout is %.2f sec", time.Since(start).Seconds())
	return nil
}
