/*
 * Created At: 2022/09/27
 * Created by Kevin(k9982874.gmail). All rights reserved.
 * Reference to the project dnsproxy(github.com/AdguardTeam/dnsproxy)
 *
 * Please distribute this file under the GNU General Public License.
 */
package dnscrypt

import (
	"crypto/rand"
	"encoding/binary"
	"io"

	v2 "github.com/ameshkov/dnscrypt/v2"
	"github.com/ameshkov/dnscrypt/v2/xsecretbox"
	"github.com/miekg/dns"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

const (
	// See 11. Authenticated encryption and key exchange algorithm
	// The public and secret keys are 32 bytes long in storage
	keySize = 32

	// size of the shared key used to encrypt/decrypt messages
	sharedKeySize = 32
)

// generateRandomKeyPair generates a random key-pair
func generateRandomKeyPair() (privateKey [keySize]byte, publicKey [keySize]byte) {
	privateKey = [keySize]byte{}
	publicKey = [keySize]byte{}

	_, _ = rand.Read(privateKey[:])
	curve25519.ScalarBaseMult(&publicKey, &privateKey)
	return
}

// computeSharedKey - computes a shared key
func computeSharedKey(cryptoConstruction v2.CryptoConstruction, secretKey *[keySize]byte, publicKey *[keySize]byte) ([keySize]byte, error) {
	if cryptoConstruction == v2.XChacha20Poly1305 {
		sharedKey, err := xsecretbox.SharedKey(*secretKey, *publicKey)
		if err != nil {
			return sharedKey, err
		}
		return sharedKey, nil
	} else if cryptoConstruction == v2.XSalsa20Poly1305 {
		sharedKey := [sharedKeySize]byte{}
		box.Precompute(&sharedKey, publicKey, secretKey)
		return sharedKey, nil
	}
	return [keySize]byte{}, v2.ErrEsVersion
}

// readPrefixed -- reads a DNS message with a 2-byte prefix containing message length
func readPrefixed(r io.Reader) (int, []byte, error) {
	l := make([]byte, 2)
	_, err := r.Read(l)
	if err != nil {
		return 0, nil, err
	}
	packetLen := binary.BigEndian.Uint16(l)
	if packetLen > dns.MaxMsgSize {
		return 0, nil, v2.ErrQueryTooLarge
	}

	buf := make([]byte, packetLen)

	n, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, nil, err
	}
	return n, buf, nil
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func dddToByte(s []byte) byte {
	return (s[0]-'0')*100 + (s[1]-'0')*10 + (s[2] - '0')
}

func unpackTxtString(s string) ([]byte, error) {
	bs := make([]byte, len(s))
	msg := make([]byte, 0)
	copy(bs, s)
	for i := 0; i < len(bs); i++ {
		if bs[i] == '\\' {
			i++
			if i == len(bs) {
				break
			}
			if i+2 < len(bs) && isDigit(bs[i]) && isDigit(bs[i+1]) && isDigit(bs[i+2]) {
				msg = append(msg, dddToByte(bs[i:]))
				i += 2
			} else if bs[i] == 't' {
				msg = append(msg, '\t')
			} else if bs[i] == 'r' {
				msg = append(msg, '\r')
			} else if bs[i] == 'n' {
				msg = append(msg, '\n')
			} else {
				msg = append(msg, bs[i])
			}
		} else {
			msg = append(msg, bs[i])
		}
	}
	return msg, nil
}
