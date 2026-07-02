package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

func GenerateKeyPair() (*ecdh.PrivateKey, error) {
	return ecdh.P256().GenerateKey(rand.Reader)
}

func PublicKeyToBase64(key *ecdh.PublicKey) (string, error) {
	return base64.RawURLEncoding.EncodeToString(key.Bytes()), nil
}

func DeriveSessionKey(privateKey *ecdh.PrivateKey, peerPublicKeyBase64 string) ([]byte, error) {
	peerRaw, err := base64.RawURLEncoding.DecodeString(peerPublicKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode peer public key: %w", err)
	}
	peerKey, err := ecdh.P256().NewPublicKey(peerRaw)
	if err != nil {
		return nil, fmt.Errorf("parse peer public key: %w", err)
	}
	shared, err := privateKey.ECDH(peerKey)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}
	h := hkdf.New(sha256.New, shared, nil, []byte("itsasecret-session-key"))
	sessionKey := make([]byte, 32)
	if _, err := io.ReadFull(h, sessionKey); err != nil {
		return nil, fmt.Errorf("hkdf: %w", err)
	}
	return sessionKey, nil
}
