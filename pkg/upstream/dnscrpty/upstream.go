/*
 * Created At: 2022/09/26
 * Created by Kevin(k9982874.gmail). All rights reserved.
 * Reference to the project dnsproxy(github.com/AdguardTeam/dnsproxy)
 *
 * Please distribute this file under the GNU General Public License.
 */

package dnscrpty

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"github.com/ameshkov/dnscrypt/v2"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const (
	defaultTimeout = time.Second * 5
)

var (
	nopLogger = zap.NewNop()
)

type Opts struct {
	// Nil logger disables logging.
	Logger *zap.Logger

	URL *url.URL

	// Timeout is the default upstream timeout. Also, it is used as a timeout for bootstrap DNS requests.
	// timeout=0 means infinite timeout.
	Timeout time.Duration

	// VerifyDNSCryptCertificate is callback to which the DNSCrypt server certificate will be passed.
	// is called in dnsCrypt.exchangeDNSCrypt; if error != nil then Upstream.Exchange() will return it
	VerifyDNSCryptCertificate func(cert *dnscrypt.Cert) error
}

func (opts *Opts) init() error {
	if opts.Logger == nil {
		opts.Logger = nopLogger
	}

	// if opts.DialFunc == nil || opts.WriteFunc == nil || opts.ReadFunc == nil {
	// 	return errors.New("opts missing required func(s)")
	// }

	utils.SetDefaultNum(&opts.Timeout, defaultTimeout)
	return nil
}

//
// DNSCrypt
//
type Upstream struct {
	opts       *Opts
	client     *dnscrypt.Client       // DNSCrypt client properties
	serverInfo *dnscrypt.ResolverInfo // DNSCrypt resolver info

	sync.RWMutex // protects DNSCrypt client
}

func NewDNSCrpty(opts Opts) (*Upstream, error) {
	if err := opts.init(); err != nil {
		return nil, err
	}

	return &Upstream{
		opts: &opts,
	}, nil
}

func (u *Upstream) CloseIdleConnections() {
}

func (u *Upstream) Close() error {
	u.Lock()
	u.client = nil
	u.serverInfo = nil
	u.Unlock()

	return nil
}

func (u *Upstream) Address() string { return u.opts.URL.String() }

func (u *Upstream) ExchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	// wire, buf, err := pool.PackBuffer(q)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to pack query msg, %w", err)
	// }
	// defer buf.Release()

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

		r, err := u.exchangeDNSCrypt(q)
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
			u.Lock()
			u.client = nil
			u.serverInfo = nil
			u.Unlock()
		}

		return r, err
	}
}

// exchangeDNSCrypt attempts to send the DNS query and returns the response
func (u *Upstream) exchangeDNSCrypt(m *dns.Msg) (*dns.Msg, error) {
	var client *dnscrypt.Client
	var resolverInfo *dnscrypt.ResolverInfo

	u.RLock()
	client = u.client
	resolverInfo = u.serverInfo
	u.RUnlock()

	now := uint32(time.Now().Unix())
	if client == nil || resolverInfo == nil || resolverInfo.ResolverCert.NotAfter < now {
		u.Lock()

		// Using "udp" for DNSCrypt upstreams by default
		client = &dnscrypt.Client{Timeout: u.opts.Timeout}
		ri, err := client.Dial(u.Address())
		if err != nil {
			u.Unlock()
			return nil, errors.New("failed to fetch certificate info from " + u.Address())
		}

		if u.opts.VerifyDNSCryptCertificate != nil {
			err = u.opts.VerifyDNSCryptCertificate(ri.ResolverCert)
		}
		if err != nil {
			u.Unlock()
			return nil, errors.New("failed to verify certificate info from " + u.Address())
		}

		u.client = client
		u.serverInfo = ri
		resolverInfo = ri
		u.Unlock()
	}

	reply, err := client.Exchange(m, resolverInfo)

	if reply != nil && reply.Truncated {
		// log.Tracef("Truncated message was received, retrying over TCP, question: %s", m.Question[0].String())
		tcpClient := dnscrypt.Client{Timeout: u.opts.Timeout, Net: "tcp"}
		reply, err = tcpClient.Exchange(m, resolverInfo)
	}

	if err == nil && reply != nil && reply.Id != m.Id {
		err = dns.ErrId
	}

	return reply, err
}
