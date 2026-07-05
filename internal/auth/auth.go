package auth

import (
	"context"
	"fmt"
	"time"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/crypto"
)

type Session struct {
	Token      string
	ExpiresAt  time.Time
	SessionKey []byte
	// WrappedOrgKeys are the orgs' keys wrapped under the user's
	// master-password-derived key - safe to persist, useless without the
	// master password.
	WrappedOrgKeys map[string]string
}

func Login(ctx context.Context, client *api.Client, email, password string) (*Session, error) {
	privKey, err := crypto.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate ecdh keypair: %w", err)
	}
	clientPubB64, err := crypto.PublicKeyToBase64(privKey.PublicKey())
	if err != nil {
		return nil, fmt.Errorf("encode client public key: %w", err)
	}
	resp, err := client.Login(ctx, email, password, clientPubB64)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	sessionKey, err := crypto.DeriveSessionKey(privKey, resp.ServerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("derive session key: %w", err)
	}
	return &Session{
		Token:          resp.Token,
		ExpiresAt:      resp.SessionExpiresAt,
		SessionKey:     sessionKey,
		WrappedOrgKeys: resp.MasterWrappedOrgKeys,
	}, nil
}
