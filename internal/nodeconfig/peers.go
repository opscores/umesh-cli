package nodeconfig

import (
	"sort"
	"strings"
)

// peersNormalized returns the comma-separated peer list trimmed of whitespace,
// empty entries, and duplicates, sorted for stable output.
func peersNormalized(list string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(list, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// MergePeerID merges newID into a comma-separated peer list. It handles
// whitespace trimming, deduplication, and empty entries, and returns a stable,
// sorted, comma-joined result. Mirrors lib-peers.sh merge_peer_id.
func MergePeerID(current, newID string) string {
	if current == "" {
		return newID
	}
	combined := current
	if !strings.Contains(","+current+",", ","+newID+",") && !hasPeer(combined, newID) {
		combined = current + "," + newID
	}
	return strings.Join(peersNormalized(combined), ",")
}

// hasPeer reports whether an exact peer token is already present in a list.
func hasPeer(list, peer string) bool {
	for _, p := range peersNormalized(list) {
		if p == peer {
			return true
		}
	}
	return false
}
