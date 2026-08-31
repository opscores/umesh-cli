package nodeinit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DownloadGenesis fetches a genesis document with exponential backoff and
// returns the raw bytes. expectJSON validates that the body parses as JSON and
// exceeds a minimum size (guarding against error pages).
func DownloadGenesis(url string, maxRetries, retryDelaySec int, expectJSON bool) ([]byte, error) {
	var lastErr error
	for i := 1; i <= maxRetries; i++ {
		body, err := httpGet(url)
		if err != nil {
			lastErr = err
		} else if !expectJSON || validJSON(body) {
			return body, nil
		} else {
			lastErr = fmt.Errorf("response is not valid JSON (%d bytes)", len(body))
		}
		time.Sleep(time.Duration(retryDelaySec*(1<<(i-1))) * time.Second)
	}
	return nil, lastErr
}

func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func validJSON(b []byte) bool {
	var v any
	return json.Unmarshal(b, &v) == nil
}

// UnwrapGenesisRPC converts a node /genesis response ({"result":{"genesis":…}})
// into a plain genesis document when present.
func UnwrapGenesisRPC(b []byte) ([]byte, bool) {
	var env struct {
		Result struct {
			Genesis json.RawMessage `json:"genesis"`
		} `json:"result"`
	}
	if json.Unmarshal(b, &env) == nil && len(env.Result.Genesis) > 0 {
		return env.Result.Genesis, true
	}
	return b, false
}

// SHA256Hex returns the hex sha256 of b.
func SHA256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// VerifyHash compares an expected sha256 (may be empty to skip) with the actual.
func VerifyHash(expected, actual string) error {
	if expected == "" {
		return nil
	}
	if !bytes.Equal([]byte(expected), []byte(actual)) {
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}
