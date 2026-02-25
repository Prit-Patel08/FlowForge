package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

var masterKey []byte

// ResetForTests clears cached encryption state.
// Intended for tests that need deterministic key/env behavior.
func ResetForTests() {
	masterKey = nil
}

// Init loads the master key from environment variables.
func Init() error {
	// 1. Check Env Var
	envKey := os.Getenv("FLOWFORGE_MASTER_KEY")
	if envKey != "" {
		key, err := hex.DecodeString(envKey)
		if err != nil {
			return fmt.Errorf("invalid FLOWFORGE_MASTER_KEY hex: %v", err)
		}
		if len(key) != 32 {
			return fmt.Errorf("FLOWFORGE_MASTER_KEY must be 32 bytes (64 hex chars)")
		}
		masterKey = key
		return nil
	}

	return fmt.Errorf("FLOWFORGE_MASTER_KEY environment variable is NOT set; security policy requires an explicit master key for encryption")
}

// Encrypt encrypts plain text using AES-GCM.
// Returns hex encoded string: nonce + ciphertext + tag.
func Encrypt(plaintext string) (string, error) {
	if masterKey == nil {
		if err := Init(); err != nil {
			return "", err
		}
		// Double check if Init failed to set masterKey
		if masterKey == nil {
			return "", fmt.Errorf("encryption key not initialized")
		}
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// Decrypt decrypts hex encoded string.
func Decrypt(encryptedHex string) (string, error) {
	if masterKey == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}

	data, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
