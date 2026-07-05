package auth

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// TokenPrefix marks long-lived access tokens created on the website, e.g. in
// CI logs or secret scanners. The full displayed form is
// `shht_<bearer>.<transport key>`.
const TokenPrefix = "shht_"

// ParseAccessToken splits a displayed access token into its bearer half
// (sent verbatim in the Authorization header) and its transport key (which
// decrypts the org keys stored with the token's session server-side). Both
// halves are standard base64, so the first "." is an unambiguous separator.
func ParseAccessToken(raw string) (bearer string, sessionKey []byte, err error) {
	trimmed, ok := strings.CutPrefix(raw, TokenPrefix)
	if !ok {
		return "", nil, fmt.Errorf("not an access token: expected the %q prefix (create one under Tokens on the website)", TokenPrefix)
	}
	bearer, keyB64, ok := strings.Cut(trimmed, ".")
	if !ok || bearer == "" || keyB64 == "" {
		return "", nil, fmt.Errorf("malformed access token: expected %s<bearer>.<key>", TokenPrefix)
	}
	if _, err := base64.StdEncoding.DecodeString(bearer); err != nil {
		return "", nil, fmt.Errorf("malformed access token: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return "", nil, fmt.Errorf("malformed access token: %w", err)
	}
	return bearer, key, nil
}
