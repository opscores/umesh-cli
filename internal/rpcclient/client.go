// Package rpcclient is a lightweight HTTP client for the CometBFT RPC and
// Cosmos SDK REST endpoints exposed by a node (26657 / 1317).
package rpcclient

import (
	"encoding/json"
	"fmt"
	"strconv"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a single node's RPC endpoint.
type Client struct {
	// RPCBase is the base URL of the CometBFT RPC (e.g. http://127.0.0.1:26657).
	RPCBase string
	// HTTPClient is used for all requests.
	HTTPClient *http.Client
}

// New builds a client. rpcBase may be empty to skip RPC calls.
func New(rpcBase string) *Client {
	return &Client{
		RPCBase:    strings.TrimRight(rpcBase, "/"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// get performs a GET and decodes the JSON body into out. It returns a
// descriptive error on non-2xx responses.
func (c *Client) get(url string, out any) error {
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GET %s: HTTP %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

// SyncInfo describes the node's sync state, mirroring /status -> sync_info.
type SyncInfo struct {
	LatestBlockHeight string `json:"latest_block_height"`
	LatestBlockTime   string `json:"latest_block_time"`
	EarliestBlockTime string `json:"earliest_block_time"`
	CatchingUp        bool   `json:"catching_up"`
}

// NodeInfo mirrors /status -> node_info.
type NodeInfo struct {
	Moniker  string `json:"moniker"`
	Network  string `json:"network"`
	Version  string `json:"version"`
	ListenAddr string `json:"listen_addr"`
	ID       string `json:"id"`
}

// ValidatorInfo mirrors /status -> validator_info.
type ValidatorInfo struct {
	Address          string `json:"address"`
	VotingPower      string `json:"voting_power"`
	ProposerPriority int64  `json:"proposer_priority"`
}

// Status is the parsed /status response.
type Status struct {
	NodeInfo      NodeInfo      `json:"node_info"`
	SyncInfo      SyncInfo      `json:"sync_info"`
	ValidatorInfo ValidatorInfo `json:"validator_info"`
}

type statusEnvelope struct {
	Result Status `json:"result"`
}

// Status fetches and decodes /status.
func (c *Client) Status() (*Status, error) {
	var env statusEnvelope
	if err := c.get(c.RPCBase+"/status", &env); err != nil {
		return nil, err
	}
	return &env.Result, nil
}

// NetInfo reports the number of connected peers, mirroring /net_info.
type NetInfo struct {
	NPeers     int `json:"n_peers"`
}

type netInfoEnvelope struct {
	Result struct {
		NPeers string `json:"n_peers"`
	} `json:"result"`
}

// NetInfo fetches and decodes /net_info.
func (c *Client) NetInfo() (*NetInfo, error) {
	var env netInfoEnvelope
	if err := c.get(c.RPCBase+"/net_info", &env); err != nil {
		return nil, err
	}
	npeers, _ := strconv.Atoi(env.Result.NPeers)
	return &NetInfo{NPeers: npeers}, nil
}


// Health returns nil when the node responds to /health with 200.
func (c *Client) Health() error {
	resp, err := c.HTTPClient.Get(c.RPCBase + "/health")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: HTTP %d", resp.StatusCode)
	}
	return nil
}
