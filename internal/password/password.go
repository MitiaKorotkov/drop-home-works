package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type params struct {
	memory  uint32
	time    uint32
	threads uint8
	saltLen uint32
	keyLen  uint32
}

var defaultParams = params{
	memory:  19 * 1024,
	time:    2,
	threads: 1,
	saltLen: 16,
	keyLen:  32,
}

var ErrInvalidHash = errors.New("password: invalid hash format")

func Hash(password string) string {
	return hashWithParams(password, defaultParams)
}

func hashWithParams(password string, p params) string {
	salt := make([]byte, p.saltLen)
	rand.Read(salt)

	hash := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, p.keyLen)
	encodedPasswdData := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.time, p.threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash),
	)

	return encodedPasswdData
}

func Verify(password, encoded string) (bool, error) {
	// Сам хеш в сообщения об ошибках не попадает: только то, какая часть строки неверна.
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("%w: want 6 parts separated by '$', got %d", ErrInvalidHash, len(parts))
	}
	if parts[1] != "argon2id" {
		return false, fmt.Errorf("%w: unsupported algorithm %q, want argon2id", ErrInvalidHash, parts[1])
	}

	var v int
	_, err := fmt.Sscanf(parts[2], "v=%d", &v)
	if err != nil || v != argon2.Version {
		return false, fmt.Errorf("%w: unsupported version %q, want v=%d", ErrInvalidHash, parts[2], argon2.Version)
	}

	var m, t uint32
	var p uint8
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p)
	if err != nil {
		return false, fmt.Errorf("%w: parse parameters %q: %v", ErrInvalidHash, parts[3], err)
	}
	// argon2.IDKey паникует при t < 1 или p < 1.
	if t == 0 || p == 0 {
		return false, fmt.Errorf("%w: time and threads must be positive, got t=%d, p=%d", ErrInvalidHash, t, p)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("%w: decode salt: %v", ErrInvalidHash, err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("%w: decode hash: %v", ErrInvalidHash, err)
	}

	if len(salt) < 8 || len(hash) < 16 {
		return false, fmt.Errorf("%w: salt or hash too short (%d and %d bytes)", ErrInvalidHash, len(salt), len(hash))
	}

	other := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, other) == 1, nil
}
