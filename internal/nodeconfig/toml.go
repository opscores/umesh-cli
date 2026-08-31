// Package nodeconfig reads and modifies a node's config.toml and app.toml.
//
// This is the Go equivalent of the bash dasel helpers. Because pelletier
// go-toml/v2 decodes into native Go maps, dashed keys (e.g. "state-sync",
// "enabled-unsafe-cors") are just ordinary map keys here — unlike in dasel v3,
// no special get("key") escaping is required.
package nodeconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// NodeConfig wraps a node's config.toml, app.toml, and client.toml as nested maps.
type NodeConfig struct {
	Dir        string // host or container path to the config directory
	Config     *Tree  // config.toml
	App        *Tree  // app.toml
	Client     *Tree  // client.toml
	ConfigPath string
	AppPath    string
	ClientPath string
}

// Tree is a generic TOML document backed by map[string]any and its file path.
type Tree struct {
	path string
	root map[string]any
}

// NewTree wraps a map as an in-memory tree with an optional file path.
func NewTree(path string, root map[string]any) *Tree {
	if root == nil {
		root = map[string]any{}
	}
	return &Tree{path: path, root: root}
}

// Load reads config.toml, app.toml, and client.toml from dir. A missing file
// is not an error; the corresponding tree is empty.
func Load(dir string) (*NodeConfig, error) {
	nc := &NodeConfig{
		Dir:        dir,
		ConfigPath: filepath.Join(dir, "config.toml"),
		AppPath:    filepath.Join(dir, "app.toml"),
		ClientPath: filepath.Join(dir, "client.toml"),
		Config:     NewTree(filepath.Join(dir, "config.toml"), nil),
		App:        NewTree(filepath.Join(dir, "app.toml"), nil),
		Client:     NewTree(filepath.Join(dir, "client.toml"), nil),
	}
	if b, err := os.ReadFile(nc.ConfigPath); err == nil {
		if err := nc.Config.Parse(b); err != nil {
			return nil, fmt.Errorf("parse %s: %w", nc.ConfigPath, err)
		}
	}
	if b, err := os.ReadFile(nc.AppPath); err == nil {
		if err := nc.App.Parse(b); err != nil {
			return nil, fmt.Errorf("parse %s: %w", nc.AppPath, err)
		}
	}
	if b, err := os.ReadFile(nc.ClientPath); err == nil {
		if err := nc.Client.Parse(b); err != nil {
			return nil, fmt.Errorf("parse %s: %w", nc.ClientPath, err)
		}
	}
	return nc, nil
}

// Parse decodes TOML bytes into the tree root.
func (t *Tree) Parse(data []byte) error {
	return toml.Unmarshal(data, &t.root)
}

// Save writes the tree back to its file in canonical TOML. An empty path is a
// no-op (used for purely in-memory trees).
func (t *Tree) Save() error {
	if t.path == "" {
		return nil
	}
	b, err := toml.Marshal(t.root)
	if err != nil {
		return err
	}
	return os.WriteFile(t.path, b, 0o600)
}

// Get returns the value at a dot-separated path, or ok=false if absent. Keys
// containing dots are not supported (matching the node config conventions
// where all dashed keys are dash-based, not dot-based).
func (t *Tree) Get(path string) (any, bool) {
	return lookup(t.root, splitPath(path))
}

// Set puts a scalar value at a dot-path and flushes the file. Segments are
// split on '.', so keys containing dots are not supported (matching the node
// config conventions where all dashed keys are dash-based, not dot-based).
func (nc *NodeConfig) Set(tree *Tree, path string, value any) error {
	sel := splitPath(path)
	if err := setPath(tree.root, sel, value); err != nil {
		return fmt.Errorf("set %s: %w", path, err)
	}
	return tree.Save()
}

// GetString returns a scalar at dot-path as a string, or def if absent/null.
func (t *Tree) GetString(path, def string) string {
	v, ok := lookup(t.root, splitPath(path))
	if !ok || v == nil {
		return def
	}
	switch k := v.(type) {
	case string:
		return k
	case bool:
		return strconv.FormatBool(k)
	case int64:
		return strconv.FormatInt(k, 10)
	case float64:
		return strconv.FormatFloat(k, 'f', -1, 64)
	}
	return def
}

// GetBool returns a scalar at dot-path as a bool, or def if absent/null.
func (t *Tree) GetBool(path string, def bool) bool {
	v, ok := lookup(t.root, splitPath(path))
	if !ok || v == nil {
		return def
	}
	switch k := v.(type) {
	case bool:
		return k
	case string:
		return k == "true"
	}
	return def
}

// GetInt64 returns a scalar at dot-path as int64, or def if absent/null.
func (t *Tree) GetInt64(path string, def int64) int64 {
	v, ok := lookup(t.root, splitPath(path))
	if !ok || v == nil {
		return def
	}
	switch k := v.(type) {
	case int64:
		return k
	case float64:
		return int64(k)
	case string:
		i, _ := strconv.ParseInt(k, 10, 64)
		return i
	}
	return def
}

// splitPath splits a dot-path into segments.
func splitPath(path string) []string {
	var out []string
	for _, p := range strings.Split(path, ".") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// lookup walks the map following sel and returns the value at that path.
func lookup(m map[string]any, sel []string) (any, bool) {
	var cur any = m
	for _, key := range sel {
		mi, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, exists := mi[key]
		if !exists {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// setPath assigns value at sel, creating intermediate maps as needed. It
// returns an error if an intermediate segment is not a map.
func setPath(m map[string]any, sel []string, value any) error {
	if len(sel) == 0 {
		return nil
	}
	cur := m
	for i := 0; i < len(sel)-1; i++ {
		key := sel[i]
		next, ok := cur[key]
		if !ok {
			nm := map[string]any{}
			cur[key] = nm
			cur = nm
			continue
		}
		nm, ok := next.(map[string]any)
		if !ok {
			return &PathError{Key: key}
		}
		cur = nm
	}
	cur[sel[len(sel)-1]] = value
	return nil
}

// PathError indicates a path segment that cannot be descended into.
type PathError struct{ Key string }

// Error implements error.
func (e *PathError) Error() string {
	return "cannot traverse through non-map segment: " + e.Key
}
