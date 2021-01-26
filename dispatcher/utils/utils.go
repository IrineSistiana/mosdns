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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"io/ioutil"
	"math/big"
	"net"
	"regexp"
	"strconv"
	"strings"
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
	default:
		ipStr, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			return nil
		}
		return net.ParseIP(ipStr)
	}
}

// ParseAddr splits addr to protocol and host.
func ParseAddr(addr string) (protocol, host string) {
	if s := strings.SplitN(addr, "://", 2); len(s) == 2 {
		protocol = s[0]
		host = s[1]
	} else {
		host = addr
	}

	return
}

// TryAddPort add port to host if host does not has an port suffix.
func TryAddPort(host string, port uint16) string {
	if _, p, _ := net.SplitHostPort(host); len(p) == 0 {
		return host + ":" + strconv.Itoa(int(port))
	}
	return host
}

// NetAddr implements net.Addr interface.
type NetAddr struct {
	str     string
	network string
}

func NewNetAddr(str string, network string) *NetAddr {
	return &NetAddr{str: str, network: network}
}

func (n *NetAddr) Network() string {
	if len(n.network) == 0 {
		return "<nil>"
	}
	return n.network
}

func (n *NetAddr) String() string {
	if len(n.str) == 0 {
		return "<nil>"
	}
	return n.str
}

// GetMsgKey unpacks m and set its id to 0.
func GetMsgKey(m *dns.Msg) (string, error) {
	buf := make([]byte, m.Len())
	wireMsg, err := m.PackBuffer(buf)
	if err != nil {
		return "", err
	}

	wireMsg[0] = 0
	wireMsg[1] = 0
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

// SplitLine extracts words from s.
func SplitLine(s string) []string {
	return charBlockExpr.FindAllString(s, -1)
}

// RemoveComment removes comment after "symbol".
func RemoveComment(s, symbol string) string {
	return strings.SplitN(s, symbol, 2)[0]
}

type ExchangeSingleFlightGroup struct {
	singleflight.Group
}

func (g *ExchangeSingleFlightGroup) Exchange(ctx context.Context, qCtx *handler.Context, upstreams []Upstream, logger *zap.Logger) (r *dns.Msg, err error) {
	key, err := GetMsgKey(qCtx.Q())
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
	if shared && rUnsafe != nil { // shared reply may has different id and is not safe to modify.
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
		return nil, errors.New("no upstream is configured")
	}
	if t == 1 {
		u := upstreams[0]
		r, err = u.Exchange(qCtx)
		if err != nil {
			return nil, err
		}
		logger.Debug("received response", qCtx.InfoField(), zap.String("from", u.Address()))
		return r, nil
	}

	c := make(chan *parallelResult, t) // use buf chan to avoid block.
	qCopy := qCtx.Copy()               // qCtx is not safe for concurrent use.
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
				logger.Debug("untrusted upstream return an err rcode", qCtx.InfoField(), zap.String("from", res.from.Address()), zap.Int("rcode", res.r.Rcode))
				continue
			}

			logger.Debug("received response", qCtx.InfoField(), zap.String("from", res.from.Address()))
			return res.r, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// all upstreams are failed
	return nil, errors.New("no response")
}

// GetMinimalAnswerTTL returns the minimal ttl of m.Answer section.
// If msg m has no answer, it returns 0.
func GetMinimalAnswerTTL(m *dns.Msg) uint32 {
	minTTL := ^uint32(0)
	for _, r := range m.Answer {
		ttl := r.Header().Ttl
		if ttl < minTTL {
			minTTL = ttl
		}
	}

	if minTTL == ^uint32(0) { // no ttl applied
		return 0
	}
	return minTTL
}

// SetAnswerTTL sets all of the m.Answer to ttl.
func SetAnswerTTL(m *dns.Msg, ttl uint32) {
	for _, r := range m.Answer {
		r.Header().Ttl = ttl
	}
}
