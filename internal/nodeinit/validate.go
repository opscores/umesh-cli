// Package nodeinit implements role initialization inside the node container.
// It mirrors the four init-*.sh scripts. Commands are issued against the local
// umeshd binary (present in the image), never through docker.
package nodeinit

import (
	"fmt"
	"regexp"
	"strings"
)

func fileExists(p string) bool {
	_, err := osStat(p)
	return err == nil
}

// Validators for env fields, matching the regexes in the bash init scripts.
var (
	reChainID    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,50}$`)
	reMoniker    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,70}$`)
	reDenom      = regexp.MustCompile(`^[a-z][a-z0-9/]{2,127}$`)
	reAmount     = regexp.MustCompile(`^[0-9]+[a-z][a-z0-9/]{2,127}$`)
	reGasPrice   = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
	reEnv        = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	reIP         = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$`)
	reNodeID     = regexp.MustCompile(`^[a-f0-9]{40}$`)
	rePrivateIP  = regexp.MustCompile(`^(10\.|172\.(1[6-9]|2[0-9]|3[01])\.|192\.168\.)`)
)

// ValidateCommon checks the fields shared by every role and returns a
// user-facing error with the offending variable name.
func ValidateCommon(chainID, moniker, denom, minGasPrice, environment string) error {
	if !reChainID.MatchString(chainID) {
		return fmt.Errorf("invalid CHAIN_ID format: %q", chainID)
	}
	if !reMoniker.MatchString(moniker) {
		return fmt.Errorf("invalid MONIKER format: %q", moniker)
	}
	if !reDenom.MatchString(denom) {
		return fmt.Errorf("invalid DENOM format: %q", denom)
	}
	if minGasPrice != "" && !reGasPrice.MatchString(minGasPrice) {
		return fmt.Errorf("invalid MIN_GAS_PRICE format: %q", minGasPrice)
	}
	if !reEnv.MatchString(environment) {
		return fmt.Errorf("invalid ENVIRONMENT format: %q", environment)
	}
	return nil
}

// ValidateNodeID validates a 40-hex-char CometBFT node ID.
func ValidateNodeID(id string) error {
	if !reNodeID.MatchString(id) {
		return fmt.Errorf("invalid node ID format: %q (expected 40 hex chars)", id)
	}
	return nil
}

// ValidatePrivateIP checks that an IPv4 address is well-formed and, when
// strict is true, falls inside the RFC 1918 private ranges.
func ValidatePrivateIP(ip string, strict bool) error {
	parts := strings.Split(ip, ".")
	if !reIP.MatchString(ip) || len(parts) != 4 {
		return fmt.Errorf("invalid IPv4 address: %q", ip)
	}
	for _, p := range parts {
		var n int
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil || n > 255 {
			return fmt.Errorf("octet out of range in IPv4 address: %q", ip)
		}
	}
	if strict && !rePrivateIP.MatchString(ip) {
		return fmt.Errorf("%q is not a private (RFC 1918) IP address", ip)
	}
	return nil
}

// ValidateAmount validates a coin amount of the form "<number><denom>".
func ValidateAmount(amount, denom string) error {
	if !reAmount.MatchString(amount) {
		return fmt.Errorf("invalid amount format: %q (expected <number><denom>)", amount)
	}
	if !strings.HasSuffix(amount, denom) {
		return fmt.Errorf("amount %q does not use configured denom %q", amount, denom)
	}
	return nil
}
