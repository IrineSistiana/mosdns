package utils

import (
	"crypto/sha256"
	"errors"
	"net"
	"sync"
)

var quicSrkSalt = []byte{115, 189, 156, 229, 145, 216, 251, 127, 220, 89,
	243, 234, 211, 79, 190, 166, 135, 253, 183, 36, 245, 174, 78, 200, 54, 213,
	85, 255, 104, 240, 103, 27}

var (
	quicSrkInitOnce  sync.Once
	quicSrk          *[32]byte
	quicSrkFromIface net.Interface
	quicSrkInitErr   error
)

func initQUICSrkFromIfaceMac() {
	nonZero := func(b []byte) bool {
		for _, i := range b {
			if i != 0 {
				return true
			}
		}
		return false
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		quicSrkInitErr = err
		return
	}
	for _, iface := range ifaces {
		if nonZero(iface.HardwareAddr) {
			k := sha256.Sum256(append(iface.HardwareAddr, quicSrkSalt...))
			quicSrk = &k
			quicSrkFromIface = iface
			return
		}
	}
	quicSrkInitErr = errors.New("cannot find non-zero mac interface")
}

// A helper func to init quic stateless reset key.
// It use the first non-zero interface mac + sha256 hash.
func InitQUICSrkFromIfaceMac() (*[32]byte, net.Interface, error) {
	quicSrkInitOnce.Do(initQUICSrkFromIfaceMac)
	return quicSrk, quicSrkFromIface, quicSrkInitErr
}
