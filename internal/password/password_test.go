package password

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	t.Run("correct password", func(t *testing.T) {
		passwd := "qwerty"
		correct, err := Verify(passwd, Hash(passwd))
		if err != nil {
			t.Fatalf("Verify(correct password): %v", err)
		}
		if !correct {
			t.Errorf("Verify(correct password) = false, want true")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		passwd := "qwerty"
		correct, err := Verify("qwert", Hash(passwd))
		if err != nil {
			t.Fatalf("Verify(wrong password): %v, want no error: a wrong password is not a broken hash", err)
		}
		if correct {
			t.Errorf("Verify(wrong password) = true, want false")
		}
	})

	t.Run("salt is working", func(t *testing.T) {
		passwd := "qwerty"
		hashed1 := Hash(passwd)
		hashed2 := Hash(passwd)

		if hashed1 == hashed2 {
			t.Errorf("two hashes of the same password are equal (%q): the salt is not random", hashed1)
		}

		correct, err := Verify(passwd, hashed1)
		if err != nil {
			t.Fatalf("Verify(password, first hash): %v", err)
		}
		if !correct {
			t.Errorf("Verify(password, first hash) = false, want true")
		}

		correct, err = Verify(passwd, hashed2)
		if err != nil {
			t.Fatalf("Verify(password, second hash): %v", err)
		}
		if !correct {
			t.Errorf("Verify(password, second hash) = false, want true")
		}
	})

	t.Run("format", func(t *testing.T) {
		const wantPrefix = "$argon2id$v=19$m=19456,t=2,p=1$"
		hashed := Hash("qwerty")

		if !strings.HasPrefix(hashed, wantPrefix) {
			t.Errorf("Hash() = %q, want prefix %q", hashed, wantPrefix)
		}
	})

	t.Run("verify takes params from string", func(t *testing.T) {
		passwd := "qwerty"
		correct, err := Verify(passwd, hashWithParams(passwd, params{
			memory:  1024,
			time:    1,
			threads: 1,
			saltLen: 16,
			keyLen:  32,
		}))
		if err != nil {
			t.Fatalf("Verify(hash with m=1024,t=1,p=1): %v", err)
		}
		if !correct {
			t.Errorf("Verify(hash with m=1024,t=1,p=1) = false, want true: Verify must take the parameters from the string, not from defaultParams")
		}
	})

	t.Run("corrupted string", func(t *testing.T) {
		good := hashWithParams("abc", params{memory: 1024, time: 1, threads: 1, saltLen: 16, keyLen: 32})
		if ok, err := Verify("abc", good); !ok || err != nil {
			t.Fatalf("base hash is broken: Verify = %v, %v", ok, err)
		}

		replacePart := func(i int, value string) string {
			parts := strings.Split(good, "$")
			parts[i] = value
			return strings.Join(parts, "$")
		}

		tests := []struct{ name, encoded string }{
			{"empty string", ""},
			{"no separators", "plain"},
			{"missing leading $", strings.TrimPrefix(good, "$")},
			{"other algorithm", replacePart(1, "argon2i")},
			{"other version", replacePart(2, "v=18")},
			{"bad params", replacePart(3, "m=x,t=1,p=1")},
			{"zero time", replacePart(3, "m=1024,t=0,p=1")},
			{"salt is not base64", replacePart(4, "!!!")},
			{"hash is not base64", replacePart(5, "!!!")},
			{"empty salt", replacePart(4, "")},
			{"empty hash", replacePart(5, "")},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := Verify(good, tt.encoded)
				if !errors.Is(err, ErrInvalidHash) {
					t.Errorf("Verify(%q) error = %v, want %v", tt.encoded, err, ErrInvalidHash)
				}
			})
		}
	})
}
