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

func HasEncryptionKey(hexKey string) bool {
	return len(hexKey) == 64
}

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
	sealed := gcm.Seal(nil, iv, plaintext, nil)
	ct := sealed[:len(sealed)-tagLen]
	tag := sealed[len(sealed)-tagLen:]

	out := make([]byte, 0, ivLen+tagLen+len(ct))
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

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
	sealed := make([]byte, 0, len(ct)+tagLen)
	sealed = append(sealed, ct...)
	sealed = append(sealed, tag...)

	plaintext, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return fmt.Errorf("crypto: decryption failed: %w", err)
	}
	return json.Unmarshal(plaintext, out)
}
