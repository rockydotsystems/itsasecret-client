package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type KdfParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
}

func DefaultKdfParams() KdfParams {
	return KdfParams{
		Memory:      64 * 1024,
		Iterations:  3,
		Parallelism: 4,
	}
}

func DeriveKey(password string, salt []byte, params KdfParams) []byte {
	return argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, 32)
}

func HashPassword(password string, salt []byte, params KdfParams) string {
	if len(salt) == 0 {
		salt = make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			panic(err)
		}
	}
	hash := DeriveKey(password, salt, params)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		params.Memory, params.Iterations, params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func VerifyPassword(password, hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		return false
	}
	if parts[1] != "argon2id" {
		return false
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	params := KdfParams{Memory: memory, Iterations: iterations, Parallelism: parallelism}
	actual := DeriveKey(password, salt, params)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
