package gateway

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DeviceIdentity holds the persistent keypair and derived device ID.
type DeviceIdentity struct {
	DeviceID   string `json:"deviceId"`
	PublicKey  string `json:"publicKey"`  // base64url-encoded, no padding
	PrivateKey string `json:"privateKey"` // base64url-encoded, no padding
	Version    int    `json:"version"`
}

// DeviceConnectInfo is sent in connect params.
type DeviceConnectInfo struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
	SignedAt  int64  `json:"signedAt"`
	Nonce     string `json:"nonce"`
}

// LoadOrCreateDevice loads a device identity from disk, or creates a new one.
func LoadOrCreateDevice(configDir string) (*DeviceIdentity, error) {
	path := filepath.Join(configDir, "device.json")

	data, err := os.ReadFile(path)
	if err == nil {
		var identity DeviceIdentity
		if err := json.Unmarshal(data, &identity); err == nil && identity.Version == 1 {
			// Verify the device ID matches the public key
			pubBytes := b64urlDecode(identity.PublicKey)
			if len(pubBytes) == ed25519.PublicKeySize {
				expectedID := sha256Hex(pubBytes)
				if expectedID == identity.DeviceID {
					return &identity, nil
				}
				// Fix mismatched ID
				identity.DeviceID = expectedID
				_ = saveDevice(path, &identity)
				return &identity, nil
			}
		}
	}

	// Generate new keypair
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}

	identity := &DeviceIdentity{
		DeviceID:   sha256Hex([]byte(pub)),
		PublicKey:  b64urlEncode([]byte(pub)),
		PrivateKey: b64urlEncode(priv.Seed()), // store only the 32-byte seed
		Version:    1,
	}

	if err := saveDevice(path, identity); err != nil {
		return nil, err
	}

	return identity, nil
}

// SignConnect creates the device connect info with a signed payload.
func (d *DeviceIdentity) SignConnect(clientID, clientMode, role string, scopes []string, token, nonce string) *DeviceConnectInfo {
	signedAt := time.Now().UnixMilli()

	scopeStr := strings.Join(scopes, ",")
	tokenStr := ""
	if token != "" {
		tokenStr = token
	}

	// v2 payload format: v2|deviceId|clientId|clientMode|role|scopes|signedAtMs|token|nonce
	payload := fmt.Sprintf("v2|%s|%s|%s|%s|%s|%d|%s|%s",
		d.DeviceID, clientID, clientMode, role, scopeStr, signedAt, tokenStr, nonce)

	// Reconstruct the full private key from seed
	seed := b64urlDecode(d.PrivateKey)
	privKey := ed25519.NewKeyFromSeed(seed)

	sig := ed25519.Sign(privKey, []byte(payload))

	return &DeviceConnectInfo{
		ID:        d.DeviceID,
		PublicKey: d.PublicKey,
		Signature: b64urlEncode(sig),
		SignedAt:  signedAt,
		Nonce:     nonce,
	}
}

func saveDevice(path string, identity *DeviceIdentity) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create device dir: %w", err)
	}
	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal device: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func b64urlEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func b64urlDecode(s string) []byte {
	data, _ := base64.RawURLEncoding.DecodeString(s)
	return data
}
