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

package utils

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"io/ioutil"
	"math/big"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// GetIPFromAddr returns net.IP from net.Addr.
// Will return nil if no ip address can be parsed.
func GetIPFromAddr(addr net.Addr) (ip net.IP) {
	switch v := addr.(type) {
	case *net.TCPAddr:
		return v.IP
	case *net.UDPAddr:
		return v.IP
	case *net.IPNet:
		return v.IP
	case *NetAddr:
		return v.IP()
	default:
		return parseIPFromAddr(addr.String())
	}
}

// SplitSchemeAndHost splits addr to protocol and host.
func SplitSchemeAndHost(addr string) (protocol, host string) {
	if protocol, host, ok := SplitString2(addr, "://"); ok {
		return protocol, host
	} else {
		return "", addr
	}
}

// NetAddr implements net.Addr interface.
type NetAddr struct {
	addr    string
	network string

	parseIPOnce sync.Once
	ip          net.IP // will be non-nil if addr is an ip addr.
}

func NewNetAddr(addr string, network string) *NetAddr {
	return &NetAddr{addr: addr, network: network}
}

func (n *NetAddr) Network() string {
	return n.network
}

func (n *NetAddr) String() string {
	return n.addr
}

func (n *NetAddr) IP() net.IP {
	n.parseIPOnce.Do(func() {
		n.ip = parseIPFromAddr(n.addr)
	})
	return n.ip
}

func parseIPFromAddr(s string) net.IP {
	ipStr, _, err := net.SplitHostPort(s)
	if err != nil {
		return nil
	}
	return net.ParseIP(ipStr)
}

// GetMsgKey unpacks m and set its id to salt.
func GetMsgKey(m *dns.Msg, salt uint16) (string, error) {
	wireMsg, err := m.Pack()
	if err != nil {
		return "", err
	}
	wireMsg[0] = byte(salt >> 8)
	wireMsg[1] = byte(salt)
	return BytesToStringUnsafe(wireMsg), nil
}

// BytesToStringUnsafe converts bytes to string.
func BytesToStringUnsafe(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// LoadCertPool reads and loads certificates in certs.
func LoadCertPool(certs []string) (*x509.CertPool, error) {
	rootCAs := x509.NewCertPool()
	for _, cert := range certs {
		b, err := ioutil.ReadFile(cert)
		if err != nil {
			return nil, err
		}

		if ok := rootCAs.AppendCertsFromPEM(b); !ok {
			return nil, fmt.Errorf("no certificate was successfully parsed in %s", cert)
		}
	}
	return rootCAs, nil
}

// GenerateCertificate generates a ecdsa certificate with given dnsName.
// This should only use in test.
func GenerateCertificate(dnsName string) (cert tls.Certificate, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return
	}

	//serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		err = fmt.Errorf("generate serial number: %w", err)
		return
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},

		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(10, 0, 0),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return
	}
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return tls.X509KeyPair(certPEM, keyPEM)
}

var charBlockExpr = regexp.MustCompile("\\S+")

// SplitLineReg extracts words from s by using regexp "\S+".
func SplitLineReg(s string) []string {
	return charBlockExpr.FindAllString(s, -1)
}

// SplitLine removes all spaces " " and extracts words from s.
func SplitLine(s string) []string {
	t := strings.Split(s, " ")
	t2 := t[:0]
	for _, sub := range t {
		if sub != "" {
			t2 = append(t2, sub)
		}
	}
	return t2
}

// RemoveComment removes comment after "symbol".
func RemoveComment(s, symbol string) string {
	if i := strings.Index(s, symbol); i >= 0 {
		return s[:i]
	}
	return s
}

//SplitString2 split s to two parts by given symbol
func SplitString2(s, symbol string) (s1 string, s2 string, ok bool) {
	if len(symbol) == 0 {
		return "", s, true
	}
	if i := strings.Index(s, symbol); i >= 0 {
		return s[:i], s[i+len(symbol):], true
	}
	return "", "", false
}

type ExchangeSingleFlightGroup struct {
	singleflight.Group
}

func (g *ExchangeSingleFlightGroup) Exchange(ctx context.Context, qCtx *handler.Context, upstreams []Upstream, logger *zap.Logger) (r *dns.Msg, err error) {
	key, err := GetMsgKey(qCtx.Q(), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to caculate msg key, %w", err)
	}

	v, err, shared := g.Do(key, func() (interface{}, error) {
		defer g.Forget(key)
		return ExchangeParallel(ctx, qCtx, upstreams, logger)
	})

	if err != nil {
		return nil, err
	}

	rUnsafe := v.(*dns.Msg)
	if shared && rUnsafe != nil { // shared reply may has a different id and is not safe to modify.
		r = rUnsafe.Copy()
		r.Id = qCtx.Q().Id
		return r, nil
	}

	return rUnsafe, nil
}

func BoolLogic(ctx context.Context, qCtx *handler.Context, fs []handler.Matcher, logicalAND bool) (matched bool, err error) {
	if len(fs) == 0 {
		return false, nil
	}

	for _, m := range fs {
		matched, err = m.Match(ctx, qCtx)
		if err != nil {
			return false, err
		}

		if matched && !logicalAND {
			return true, nil
		}
		if !matched && logicalAND {
			return false, nil
		}
	}

	return matched, nil
}

type Upstream interface {
	// Exchange should not keep nor modify qCtx.
	Exchange(qCtx *handler.Context) (*dns.Msg, error)
	Address() string
	Trusted() bool
}

type parallelResult struct {
	r    *dns.Msg
	err  error
	from Upstream
}

func ExchangeParallel(ctx context.Context, qCtx *handler.Context, upstreams []Upstream, logger *zap.Logger) (r *dns.Msg, err error) {
	t := len(upstreams)
	if t == 0 {
		return nil, errors.New("no upstream")
	}
	if t == 1 {
		u := upstreams[0]
		r, err = u.Exchange(qCtx)
		if err != nil {
			return nil, err
		}
		logger.Debug("response received", qCtx.InfoField(), zap.String("from", u.Address()))
		return r, nil
	}

	c := make(chan *parallelResult, t) // use buf chan to avoid blocking.
	qCopy := qCtx.CopyNoR()            // qCtx is not safe for concurrent use.
	for _, u := range upstreams {
		u := u
		go func() {
			r, err := u.Exchange(qCopy)
			c <- &parallelResult{
				r:    r,
				err:  err,
				from: u,
			}
		}()
	}

	for i := 0; i < t; i++ {
		select {
		case res := <-c:
			if res.err != nil {
				logger.Warn("upstream failed", qCtx.InfoField(), zap.String("from", res.from.Address()), zap.Error(res.err))
				continue
			}

			if !res.from.Trusted() && res.r.Rcode != dns.RcodeSuccess {
				logger.Debug("untrusted upstream returned an err rcode", qCtx.InfoField(), zap.String("from", res.from.Address()), zap.Int("rcode", res.r.Rcode))
				continue
			}

			logger.Debug("response received", qCtx.InfoField(), zap.String("from", res.from.Address()))
			return res.r, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// all upstreams are failed
	return nil, errors.New("no response")
}

// IsIPAddr returns true is s is a IP address. s can contain ":port".
func IsIPAddr(s string) bool {
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return net.ParseIP(s) != nil
	}
	return net.ParseIP(host) != nil
}
