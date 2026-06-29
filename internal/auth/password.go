package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type Argon2idParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultArgon2idParams = Argon2idParams{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

var ErrInvalidHash = errors.New("auth: invalid encoded hash")

var ErrIncompatibleVersion = errors.New("auth: incompatible argon2 version")

func HashPassword(password string) (string, error) {
	return HashPasswordWithParams(password, DefaultArgon2idParams)
}

func HashPasswordWithParams(password string, p Argon2idParams) (string, error) {
	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("reading random salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

	b64salt := base64.RawStdEncoding.EncodeToString(salt)
	b64key := base64.RawStdEncoding.EncodeToString(key)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism, b64salt, b64key,
	), nil
}

func VerifyPassword(password, encoded string) (bool, error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	return subtle.ConstantTimeCompare(key, candidate) == 1, nil
}

func decodeHash(encoded string) (Argon2idParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return Argon2idParams{}, nil, nil, ErrIncompatibleVersion
	}

	var p Argon2idParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}
	p.SaltLength = uint32(len(salt))

	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil {
		return Argon2idParams{}, nil, nil, ErrInvalidHash
	}
	p.KeyLength = uint32(len(key))

	return p, salt, key, nil
}
