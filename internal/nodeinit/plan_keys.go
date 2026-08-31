package nodeinit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opscores/umesh-cli/internal/dkrcmd"
	"github.com/opscores/umesh-cli/internal/uio"
)

var keyringPassword string

// SetKeyringPass sets the keyring password for plan execution.
func SetKeyringPass(pass string) {
	keyringPassword = pass
}

// GetKeyringPass returns the current keyring password.
func GetKeyringPass() string {
	return keyringPassword
}

// resolveKey resolves an allocation to a bech32 address using the priority:
// 1. If Address is set, use it directly
// 2. If Mnemonic is set, recover key from mnemonic
// 3. If KeyName exists in keyring, use existing
// 4. Otherwise, generate new key and output mnemonic
func resolveKey(d *dkrcmd.Docker, alloc Allocation) (address string, generated bool, mnemonic string, err error) {
	if err := validateKeyName(alloc.KeyName); err != nil {
		return "", false, "", err
	}
	// Priority 1: explicit address
	if alloc.Address != "" {
		return alloc.Address, false, "", nil
	}

	// Priority 2: recover from mnemonic
	if alloc.Mnemonic != "" {
		return recoverKey(d, alloc.KeyName, alloc.Mnemonic)
	}

	// Priority 3: check if key exists in keyring
	if alloc.KeyName != "" {
		if addr, ok := keyExists(d, alloc.KeyName); ok {
			return addr, false, "", nil
		}
	}

	// Priority 4: generate new key
	return generateKey(d, alloc.KeyName)
}

// keyExists checks if a key exists in the keyring and returns its address.
func keyExists(d *dkrcmd.Docker, name string) (string, bool) {
	out, err := keysShow(d, name, keyringPassword)
	if err != nil {
		return "", false
	}
	if !strings.Contains(out, `"name":"`+name+`"`) &&
		!strings.Contains(out, `"name": "`+name+`"`) {
		return "", false
	}
	var k struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(out), &k); err != nil {
		return "", false
	}
	return k.Address, k.Address != ""
}

// recoverKey recovers a key from mnemonic.
func recoverKey(d *dkrcmd.Docker, name, mnemonic string) (string, bool, string, error) {
	if name == "" {
		return "", false, "", fmt.Errorf("key_name required for mnemonic recovery")
	}
	
	mnemonicInput := mnemonic + "\n"
	passwordInput := keyringPassword + "\n" + keyringPassword + "\n"
	fullInput := mnemonicInput + passwordInput
	
	out, err := d.RunMount(strings.NewReader(fullInput), "keys", "add", name,
		"--recover",
		"--keyring-backend", "file",
		"--keyring-dir", containerHome(d)+"/keyring",
		"--home", containerHome(d),
		"--output", "json")
	if err != nil {
		return "", false, "", fmt.Errorf("recover key %s: %w", name, err)
	}
	
	var k struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(out, &k); err != nil {
		return "", false, "", fmt.Errorf("parse recovered key output: %w", err)
	}
	
	return k.Address, false, mnemonic, nil
}

// generateKey generates a new key and returns its address.
func generateKey(d *dkrcmd.Docker, name string) (string, bool, string, error) {
	if name == "" {
		return "", false, "", fmt.Errorf("key_name required for key generation")
	}
	
	out, err := d.RunMount(strings.NewReader(keyringPassword+"\n"+keyringPassword+"\n"), "keys", "add", name,
		"--keyring-backend", "file",
		"--keyring-dir", containerHome(d)+"/keyring",
		"--home", containerHome(d),
		"--output", "json")
	if err != nil {
		return "", false, "", fmt.Errorf("generate key %s: %w", name, err)
	}
	
	var k struct {
		Address  string `json:"address"`
		Mnemonic string `json:"mnemonic"`
	}
	if err := json.Unmarshal(out, &k); err != nil {
		// Fallback: try without mnemonic field
		var k2 struct {
			Address string `json:"address"`
		}
		if err2 := json.Unmarshal(out, &k2); err2 != nil {
			return "", false, "", fmt.Errorf("parse key output: %w", err)
		}
		uio.LogWarning("Key %s generated but mnemonic not returned by umeshd", name)
		return k2.Address, true, "", nil
	}
	
	// Output mnemonic to user
	if k.Mnemonic != "" {
		uio.LogWarning("============================================")
		uio.LogWarning("MNEMONIC for key '%s' (save securely):", name)
		uio.LogWarning("%s", k.Mnemonic)
		uio.LogWarning("============================================")
		uio.LogWarning("This is the ONLY time this mnemonic is shown!")
	}
	
 	return k.Address, true, k.Mnemonic, nil
}

// validateKeyName ensures the key name is compatible with Cosmos SDK keyring.
// Key names with hyphens or special characters are rejected by the SDK.
func validateKeyName(name string) error {
	if name == "" {
		return nil
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return fmt.Errorf("invalid key name %q: only alphanumeric and underscore allowed (no hyphens)", name)
		}
	}
	return nil
}
