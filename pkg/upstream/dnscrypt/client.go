/*
 * Created At: 2022/09/26
 * Created by Kevin(k9982874.gmail). All rights reserved.
 * Reference to the project dnsproxy(github.com/AdguardTeam/dnsproxy)
 *
 * Please distribute this file under the GNU General Public License.
 */
package dnscrypt

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/IrineSistiana/mosdns/v4/pkg/upstream/transport"
	v2 "github.com/ameshkov/dnscrypt/v2"
	"github.com/miekg/dns"
)

//
// DNSCrypt
//
type client struct {
	url *url.URL

	net string // protocol (can be "udp" or "tcp", by default - "udp")

	// UDPSize is the maximum size of a DNS response (or query) this client can
	// sent or receive. If not set, we use dns.MinMsgSize by default.
	udpSize int

	dialer *dialer

	trans *transport.Transport

	resolverInfo *ResolverInfo
}

func newClient(net string, upsURL *url.URL, opts Options) (*client, error) {
	c := client{
		url: upsURL,
		net: net,
	}

	idleTimeout := time.Second * 30
	if opts.IdleTimeout > 0 {
		idleTimeout = opts.IdleTimeout
	}

	maxConn := 2
	if opts.MaxConns > 0 {
		maxConn = opts.MaxConns
	}

	dialFunc := opts.UdpDialFunc
	if net == "tcp" {
		dialFunc = opts.TcpDialFunc
	}

	to := transport.Opts{
		Logger:   opts.Logger,
		DialFunc: dialFunc,
		WriteFunc: func(w io.Writer, m *dns.Msg) (int, error) {
			return c.send(w, m)
		},
		ReadFunc: func(r io.Reader) (*dns.Msg, int, error) {
			return c.receive(r)
		},
		EnablePipeline: opts.EnablePipeline,
		MaxConns:       maxConn,
		IdleTimeout:    idleTimeout,
	}

	t, err := transport.NewTransport(to)
	if err != nil {
		return nil, fmt.Errorf("cannot init udp transport, %w", err)
	}
	c.trans = t

	return &c, nil
}

func (c *client) Address() string { return c.url.String() }

func (c *client) ExchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	resolverInfo, err := c.dialer.Dial(ctx, c.Address())
	if err != nil {
		return nil, err
	}
	c.resolverInfo = resolverInfo

	return c.trans.ExchangeContext(ctx, q)
}

func (c *client) closeIdleConnections() {
	if c.trans != nil {
		c.trans.CloseIdleConnections()
	}
}

func (c *client) close() error {
	if c.trans != nil {
		return c.trans.Close()
	}
	return nil
}

func (c *client) send(w io.Writer, m *dns.Msg) (int, error) {
	query, err := c.encrypt(m)
	if err != nil {
		return 0, err
	}

	return c.writeQuery(w, query)
}

func (c *client) receive(r io.Reader) (*dns.Msg, int, error) {
	n, b, err := c.readResponse(r)
	if err != nil {
		return nil, 0, err
	}

	res, err := c.decrypt(b)
	if err != nil {
		return nil, 0, err
	}

	return res, n, nil
}

// writeQuery writes query to the network connection
// depending on the protocol we may write a 2-byte prefix or not
func (c *client) writeQuery(w io.Writer, query []byte) (int, error) {
	// Write to the connection
	if _, ok := w.(*net.TCPConn); ok {
		l := make([]byte, 2)
		binary.BigEndian.PutUint16(l, uint16(len(query)))

		len, err := (&net.Buffers{l, query}).WriteTo(w)
		return int(len), err
	}
	return w.Write(query)
}

// readResponse reads response from the network connection
// depending on the protocol, we may read a 2-byte prefix or not
func (c *client) readResponse(r io.Reader) (int, []byte, error) {
	proto := "udp"
	if _, ok := r.(*net.TCPConn); ok {
		proto = "tcp"
	}

	if proto == "udp" {
		bufSize := c.udpSize
		if bufSize == 0 {
			bufSize = dns.MinMsgSize
		}

		response := make([]byte, bufSize)

		n, err := r.Read(response)
		if err != nil {
			return 0, nil, err
		}
		return n, response[:n], nil
	}

	// If we got here, this is a TCP connection
	// so we should read a 2-byte prefix first
	return readPrefixed(r)
}

// encrypt encrypts a DNS message using shared key from the resolver info
func (c *client) encrypt(m *dns.Msg) ([]byte, error) {
	q := v2.EncryptedQuery{
		EsVersion:   c.resolverInfo.ResolverCert.EsVersion,
		ClientMagic: c.resolverInfo.ResolverCert.ClientMagic,
		ClientPk:    c.resolverInfo.PublicKey,
	}
	query, err := m.Pack()
	if err != nil {
		return nil, err
	}
	b, err := q.Encrypt(query, c.resolverInfo.SharedKey)
	if len(b) > c.maxQuerySize() {
		return nil, v2.ErrQueryTooLarge
	}

	return b, err
}

// decrypts decrypts a DNS message using a shared key from the resolver info
func (c *client) decrypt(b []byte) (*dns.Msg, error) {
	dr := v2.EncryptedResponse{
		EsVersion: c.resolverInfo.ResolverCert.EsVersion,
	}
	msg, err := dr.Decrypt(b, c.resolverInfo.SharedKey)
	if err != nil {
		return nil, err
	}

	res := new(dns.Msg)
	err = res.Unpack(msg)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *client) maxQuerySize() int {
	if c.net == "tcp" {
		return dns.MaxMsgSize
	}

	if c.udpSize > 0 {
		return c.udpSize
	}

	return dns.MinMsgSize
}
