package auth

import (
	"context"
	"fmt"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/crypto"
)

type Session struct {
	Token      string
	SessionKey []byte
	OrgKeys    map[string][]byte
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
	orgKeys := make(map[string][]byte, len(resp.WrappedOrgKeys))
	for orgID, wrapped := range resp.WrappedOrgKeys {
		key, err := crypto.UnwrapKey(sessionKey, wrapped)
		if err != nil {
			return nil, fmt.Errorf("unwrap org key %s: %w", orgID, err)
		}
		orgKeys[orgID] = key
	}
	return &Session{
		Token:      resp.Token,
		SessionKey: sessionKey,
		OrgKeys:    orgKeys,
	}, nil
}
