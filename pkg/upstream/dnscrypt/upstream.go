/*
 * Created At: 2022/09/27
 * Created by Kevin(k9982874.gmail). All rights reserved.
 * Reference to the project dnsproxy(github.com/AdguardTeam/dnsproxy)
 *
 * Please distribute this file under the GNU General Public License.
 */
package dnscrypt

import (
	"context"
	"io"
	"net"
	"net/url"
	"os"
	"time"

	v2 "github.com/ameshkov/dnscrypt/v2"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const defaultTimeout = time.Second * 5

type Options struct {
	// Nil logger disables logging.
	Logger *zap.Logger

	// VerifyDNSCryptCertificate is callback to which the DNSCrypt server certificate will be passed.
	// is called in dnsCrypt.exchangeDNSCrypt; if error != nil then Upstream.Exchange() will return it
	VerifyDNSCryptCertificate func(cert *v2.Cert) error

	// The following funcs cannot be nil.
	// DialFunc specifies the method to dial a connection to the server.
	UdpDialFunc func(ctx context.Context) (net.Conn, error)
	TcpDialFunc func(ctx context.Context) (net.Conn, error)

	// IdleTimeout controls the maximum idle time for each connection.
	// If IdleTimeout < 0, Transport will not reuse connections.
	// Default is defaultIdleTimeout.
	IdleTimeout time.Duration

	// If EnablePipeline is set and IdleTimeout > 0, the Transport will pipeline
	// queries as RFC 7766 6.2.1.1 suggested.
	EnablePipeline bool

	// MaxConns controls the maximum pipeline connections Transport can open.
	// It includes dialing connections.
	// Default is defaultMaxConns.
	// Each connection can handle no more than 65535 queries concurrently.
	// Typically, it is very rare reaching that limit.
	MaxConns int
}

type upstream struct {
	// Nil logger disables logging.
	logger *zap.Logger

	// Resolve the server information from upstream
	dialer *dialer

	// DNSCrypt client that sends udp requests.
	udpClient *client

	// DNSCrypt client that sends tcp requests.
	tcpClient *client
}

func NewDnscrypt(upsURL *url.URL, opts Options) (*upstream, error) {
	dialer, err := newDialer(opts)
	if err != nil {
		return nil, err
	}

	udpClient, err := newClient("udp", upsURL, opts)
	if err != nil {
		return nil, err
	}

	tcpClient, err := newClient("tcp", upsURL, opts)
	if err != nil {
		return nil, err
	}

	dialer.VerifyDNSCryptCertificate = opts.VerifyDNSCryptCertificate

	udpClient.dialer = dialer
	tcpClient.dialer = dialer

	return &upstream{
		logger:    opts.Logger,
		dialer:    dialer,
		udpClient: udpClient,
		tcpClient: tcpClient,
	}, nil
}

func (u *upstream) CloseIdleConnections() {
	u.dialer.closeIdleConnections()

	u.udpClient.closeIdleConnections()
	u.tcpClient.closeIdleConnections()
}

func (u *upstream) Close() error {
	u.dialer.close()

	u.udpClient.close()
	u.tcpClient.close()

	return nil
}

func (u *upstream) ExchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	type result struct {
		r   *dns.Msg
		err error
	}

	resChan := make(chan *result, 1)
	go func(ctx context.Context, q *dns.Msg) {
		// We overwrite the ctx with a fixed timout context here.
		// Because the http package may close the underlay connection
		// if the context is done before the query is completed. This
		// reduces the connection reuse efficiency.
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		r, err := u.exchangeDNSCrypt(ctx, q)
		resChan <- &result{r: r, err: err}
	}(ctx, q)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resChan:
		r := res.r
		err := res.err
		if r != nil {
			r.Id = q.Id
		}

		if os.IsTimeout(err) || err == io.EOF {
			u.dialer.serverInfo = nil
		}

		return r, err
	}
}

func (u *upstream) exchangeDNSCrypt(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	reply, err := u.udpClient.ExchangeContext(ctx, q)
	if reply != nil && reply.Truncated {
		u.logger.Debug("truncated message received, retrying over tcp, question:", zap.Any("id", q.Question[0]), zap.Error(err))
		reply, err = u.tcpClient.ExchangeContext(ctx, q)
	}

	if err == nil && reply != nil && reply.Id != q.Id {
		err = dns.ErrId
	}
	return reply, err
}
