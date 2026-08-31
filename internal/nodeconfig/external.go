package nodeconfig

import (
	"net"
	"strings"
)

// DefaultP2PPort is the default CometBFT P2P listen/advertise port.
const DefaultP2PPort = "26656"

// NormalizeExternalAddress normalizes a p2p.external_address value to
// "host:port" form without a scheme. CometBFT rejects addresses with a
// "tcp://" prefix or without a port during config validation, so a bare IP
// or hostname gets the default P2P port appended. An empty value stays empty.
func NormalizeExternalAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	addr = strings.TrimPrefix(addr, "tcp://")
	if addr == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(addr, DefaultP2PPort)
}
