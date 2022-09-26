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
	"crypto/ed25519"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/golibs/log"
	"github.com/IrineSistiana/mosdns/v4/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v4/pkg/upstream/transport"
	v2 "github.com/ameshkov/dnscrypt/v2"
	"github.com/ameshkov/dnsstamps"
	"github.com/miekg/dns"
)

// ResolverInfo contains DNSCrypt resolver information necessary for decryption/encryption
type ResolverInfo struct {
	SecretKey [keySize]byte // Client short-term secret key
	PublicKey [keySize]byte // Client short-term public key

	ServerPublicKey ed25519.PublicKey // Resolver public key (this key is used to validate cert signature)
	ServerAddress   string            // Server IP address
	ProviderName    string            // Provider name

	ResolverCert *v2.Cert      // Certificate info (obtained with the first unencrypted DNS request)
	SharedKey    [keySize]byte // Shared key that is to be used to encrypt/decrypt messages
}

type dialer struct {
	sync.RWMutex // protects dialer

	ut *transport.Transport
	tt *transport.Transport

	serverInfo *ResolverInfo // DNSCrypt resolver info

	// VerifyDNSCryptCertificate is callback to which the DNSCrypt server certificate will be passed.
	// is called in dnsCrypt.exchangeDNSCrypt; if error != nil then Upstream.Exchange() will return it
	VerifyDNSCryptCertificate func(cert *v2.Cert) error
}

func newDialer(opts Options) (*dialer, error) {
	uto := transport.Opts{
		Logger:    opts.Logger,
		DialFunc:  opts.UdpDialFunc,
		WriteFunc: dnsutils.WriteMsgToUDP,
		ReadFunc: func(c io.Reader) (*dns.Msg, int, error) {
			return dnsutils.ReadMsgFromUDP(c, 4096)
		},
		EnablePipeline: true,
		MaxConns:       opts.MaxConns,
		IdleTimeout:    time.Second * 60,
	}
	ut, err := transport.NewTransport(uto)
	if err != nil {
		return nil, fmt.Errorf("cannot init udp transport, %w", err)
	}

	tto := transport.Opts{
		Logger:    opts.Logger,
		DialFunc:  opts.TcpDialFunc,
		WriteFunc: dnsutils.WriteMsgToTCP,
		ReadFunc:  dnsutils.ReadMsgFromTCP,
	}
	tt, err := transport.NewTransport(tto)
	if err != nil {
		return nil, fmt.Errorf("cannot init tcp transport, %w", err)
	}

	return &dialer{
		ut: ut,
		tt: tt,
	}, nil
}

func (d *dialer) closeIdleConnections() {
	d.ut.CloseIdleConnections()
	d.tt.CloseIdleConnections()
}

func (d *dialer) close() {
	d.Lock()
	defer d.Unlock()

	d.ut.Close()
	d.tt.Close()

	d.serverInfo = nil
}

func (d *dialer) exchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	m, err := d.ut.ExchangeContext(ctx, q)
	if err != nil {
		return nil, err
	}
	if m.Truncated {
		return d.tt.ExchangeContext(ctx, q)
	}
	return m, nil
}

// Dial fetches and validates DNSCrypt certificate from the given server
// Data received during this call is then used for DNS requests encryption/decryption
// stampStr is an sdns:// address which is parsed using go-dnsstamps package
func (d *dialer) Dial(ctx context.Context, stampStr string) (*ResolverInfo, error) {
	d.RLock()
	resolverInfo := d.serverInfo
	d.RUnlock()

	now := uint32(time.Now().Unix())

	if resolverInfo == nil || resolverInfo.ResolverCert.NotAfter < now {
		d.Lock()
		defer d.Unlock()

		stamp, err := dnsstamps.NewServerStampFromString(stampStr)
		if err != nil {
			// Invalid SDNS stamp
			return nil, err
		}

		if stamp.Proto != dnsstamps.StampProtoTypeDNSCrypt {
			return nil, v2.ErrInvalidDNSStamp
		}

		resolverInfo, err = d.DialStamp(ctx, stamp)
		if err != nil {
			return nil, err
		}

		if d.VerifyDNSCryptCertificate != nil {
			err = d.VerifyDNSCryptCertificate(resolverInfo.ResolverCert)
			if err != nil {
				return nil, fmt.Errorf("verifying certificate info from %s: %w", stampStr, err)
			}
		}

		d.serverInfo = resolverInfo
	}

	return d.serverInfo, nil
}

// DialStamp fetches and validates DNSCrypt certificate from the given server
// Data received during this call is then used for DNS requests encryption/decryption
func (d *dialer) DialStamp(ctx context.Context, stamp dnsstamps.ServerStamp) (*ResolverInfo, error) {
	resolverInfo := &ResolverInfo{}

	// Generate the secret/public pair
	resolverInfo.SecretKey, resolverInfo.PublicKey = generateRandomKeyPair()

	// Set the provider properties
	resolverInfo.ServerPublicKey = stamp.ServerPk
	resolverInfo.ServerAddress = stamp.ServerAddrStr
	resolverInfo.ProviderName = stamp.ProviderName

	cert, err := d.fetchCert(ctx, stamp)
	if err != nil {
		return nil, err
	}
	resolverInfo.ResolverCert = cert

	// Compute shared key that we'll use to encrypt/decrypt messages
	sharedKey, err := computeSharedKey(cert.EsVersion, &resolverInfo.SecretKey, &cert.ResolverPk)
	if err != nil {
		return nil, err
	}
	resolverInfo.SharedKey = sharedKey
	return resolverInfo, nil
}

// fetchCert loads DNSCrypt cert from the specified server
func (d *dialer) fetchCert(ctx context.Context, stamp dnsstamps.ServerStamp) (*v2.Cert, error) {
	providerName := stamp.ProviderName
	if !strings.HasSuffix(providerName, ".") {
		providerName = providerName + "."
	}

	query := new(dns.Msg)
	query.SetQuestion(providerName, dns.TypeTXT)

	r, err := d.exchangeContext(ctx, query)
	if err != nil {
		return nil, err
	}

	if r.Rcode != dns.RcodeSuccess {
		return nil, v2.ErrFailedToFetchCert
	}

	var certErr error
	currentCert := &v2.Cert{}
	foundValid := false

	for _, rr := range r.Answer {
		txt, ok := rr.(*dns.TXT)
		if !ok {
			continue
		}
		var b []byte
		b, certErr = unpackTxtString(strings.Join(txt.Txt, ""))
		if certErr != nil {
			log.Debug("[%s] failed to pack TXT record: %v", providerName, certErr)
			continue
		}

		cert := &v2.Cert{}
		certErr = cert.Deserialize(b)
		if certErr != nil {
			log.Debug("[%s] failed to deserialize cert: %v", providerName, certErr)
			continue
		}

		log.Debug("[%s] fetched certificate %d", providerName, cert.Serial)

		if !cert.VerifyDate() {
			certErr = v2.ErrInvalidDate
			log.Debug("[%s] cert %d date is not valid", providerName, cert.Serial)
			continue
		}

		if !cert.VerifySignature(stamp.ServerPk) {
			certErr = v2.ErrInvalidCertSignature
			log.Debug("[%s] cert %d signature is not valid", providerName, cert.Serial)
			continue
		}

		if cert.Serial < currentCert.Serial {
			log.Debug("[%v] cert %d superseded by a previous certificate", providerName, cert.Serial)
			continue
		}

		if cert.Serial == currentCert.Serial {
			if cert.EsVersion > currentCert.EsVersion {
				log.Debug("[%v] Upgrading the construction from %v to %v", providerName, currentCert.EsVersion, cert.EsVersion)
			} else {
				log.Debug("[%v] Keeping the previous, preferred crypto construction", providerName)
				continue
			}
		}

		// Setting the cert
		currentCert = cert
		foundValid = true
	}

	if foundValid {
		return currentCert, nil
	}

	return nil, certErr
}
