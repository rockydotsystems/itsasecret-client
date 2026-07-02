package crypto

import (
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
		Iterations:  1,
		Parallelism: 1,
	}
}

// DeriveKey runs Argon2id with the supplied parameters. The CLI currently
// sends the password to the server for authentication, so this is used for
// future client-side derivation or local tooling, not for login.
func DeriveKey(password string, salt []byte, params KdfParams) []byte {
	return argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, 32)
}
