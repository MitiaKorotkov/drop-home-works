package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"time"
)

const (
	CookieName = "session"
	Lifetime   = 30 * 24 * time.Hour
)

func NewToken() (token string, hash []byte) {
	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	token = base64.RawURLEncoding.EncodeToString(randomBytes)

	return token, HashToken(token)
}

func HashToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func Cookie(token string) *http.Cookie {
	cookie := http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(Lifetime.Seconds()),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	return &cookie
}

func ClearCookie() *http.Cookie {
	cookie := http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	return &cookie
}
