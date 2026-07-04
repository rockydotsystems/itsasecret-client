package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm new: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("rand nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm new: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("gcm open: %w", err)
	}
	return plaintext, nil
}

func EncryptString(key []byte, plaintext string) (string, error) {
	ct, err := Encrypt(key, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

func DecryptString(key []byte, encoded string) (string, error) {
	ct, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	pt, err := Decrypt(key, ct)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func GenerateKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return key
}

// WrapKey mirrors the server's envelope.wrapKey: the key bytes are base64
// (std) encoded before being AES-GCM sealed, and the ciphertext is std-base64
// for the wire.
func WrapKey(wrappingKey, keyToWrap []byte) (string, error) {
	inner := base64.StdEncoding.EncodeToString(keyToWrap)
	ct, err := Encrypt(wrappingKey, []byte(inner))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

func UnwrapKey(wrappingKey []byte, wrappedKey string) ([]byte, error) {
	ct, err := base64.StdEncoding.DecodeString(wrappedKey)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	inner, err := Decrypt(wrappingKey, ct)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(string(inner))
}
