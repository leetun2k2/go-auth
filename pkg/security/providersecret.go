package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// EncryptProviderSecret encrypts a provider client_secret for storage in
// clients.client_secret_enc, using AES-256-GCM with
// cfg.Auth.ProviderSecretKey (base64-encoded 32-byte key).
func EncryptProviderSecret(base64Key, plaintext string) (string, error) {
	gcm, err := newProviderSecretGCM(base64Key)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptProviderSecret reverses EncryptProviderSecret.
func DecryptProviderSecret(base64Key, encoded string) (string, error) {
	gcm, err := newProviderSecretGCM(base64Key)
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, encrypted := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

func newProviderSecretGCM(base64Key string) (cipher.AEAD, error) {
	if base64Key == "" {
		return nil, errors.New("auth.providerSecretKey is required")
	}

	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("decode auth.providerSecretKey: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("auth.providerSecretKey must decode to 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}

	return cipher.NewGCM(block)
}
