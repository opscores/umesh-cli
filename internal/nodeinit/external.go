package nodeinit

import (
	"net"

	"github.com/opscores/umesh-cli/internal/nodeconfig"
)

// setExternalAddress writes p2p.external_address into config.toml. addr may
// be a bare IP/hostname or "host:port"; it is normalized to "host:port"
// (P2P port 26656 by default, no scheme) before being written, matching what
// CometBFT's config validation accepts.
func setExternalAddress(addr string) error {
	addr = nodeconfig.NormalizeExternalAddress(addr)
	if addr == "" {
		return nil
	}
	nc, err := nodeconfig.Load(ConfigDir())
	if err != nil {
		return err
	}
	return nc.Set(nc.Config, "p2p.external_address", addr)
}

// joinHostPort joins host and optional port, defaulting to the P2P port.
func joinHostPort(host, port string) string {
	if port == "" {
		port = nodeconfig.DefaultP2PPort
	}
	return net.JoinHostPort(host, port)
}
