// Package crypto implements AES-256-GCM encryption byte-compatible with
// apps/api/src/common/crypto.ts. Node lays out ciphertext as
// base64(iv(12) ‖ tag(16) ‖ ciphertext); Go's cipher.AEAD.Seal appends the
// tag *after* the ciphertext, so we must manually slice and reassemble.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	ivLen  = 12
	tagLen = 16
)

// keyFromHex parses a 32-byte hex-encoded key, matching Node's
// `Buffer.from(hex, 'hex')` with the same 64-hex-char length check.
func keyFromHex(hexKey string) ([]byte, error) {
	if len(hexKey) != 64 {
		return nil, errors.New("CONFIG_ENCRYPTION_KEY must be a 32-byte hex string (openssl rand -hex 32)")
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("CONFIG_ENCRYPTION_KEY is not valid hex: %w", err)
	}
	return key, nil
}

// HasEncryptionKey mirrors hasEncryptionKey() in crypto.ts.
func HasEncryptionKey(hexKey string) bool {
	return len(hexKey) == 64
}

// EncryptJSON marshals value to JSON, encrypts it with AES-256-GCM and returns
// base64(iv ‖ tag ‖ ciphertext) — the exact layout Node's encryptJson produces.
func EncryptJSON(hexKey string, value any) (string, error) {
	key, err := keyFromHex(hexKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, ivLen)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	plaintext, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	// Go's Seal appends dst ‖ ciphertext ‖ tag. Slice tag off the end and
	// reorder to iv ‖ tag ‖ ciphertext to match Node's Buffer.concat order.
	sealed := gcm.Seal(nil, iv, plaintext, nil)
	ct := sealed[:len(sealed)-tagLen]
	tag := sealed[len(sealed)-tagLen:]

	out := make([]byte, 0, ivLen+tagLen+len(ct))
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptJSON reverses EncryptJSON / Node's decryptJson: parse
// base64(iv(12) ‖ tag(16) ‖ ciphertext), decrypt, unmarshal into out.
func DecryptJSON(hexKey string, payload string, out any) error {
	key, err := keyFromHex(hexKey)
	if err != nil {
		return err
	}
	buf, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return fmt.Errorf("crypto: invalid base64 payload: %w", err)
	}
	if len(buf) < ivLen+tagLen {
		return errors.New("crypto: payload too short")
	}
	iv := buf[:ivLen]
	tag := buf[ivLen : ivLen+tagLen]
	ct := buf[ivLen+tagLen:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	// Go expects ciphertext ‖ tag for Open; Node stored tag before ciphertext.
	sealed := make([]byte, 0, len(ct)+tagLen)
	sealed = append(sealed, ct...)
	sealed = append(sealed, tag...)

	plaintext, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return fmt.Errorf("crypto: decryption failed: %w", err)
	}
	return json.Unmarshal(plaintext, out)
}
