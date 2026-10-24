package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setAndParse(t *testing.T, c *http.Cookie) *http.Cookie {
	t.Helper()

	rec := httptest.NewRecorder()
	http.SetCookie(rec, c)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies in Set-Cookie, want 1", len(cookies))
	}

	return cookies[0]
}

func TestNewToken(t *testing.T) {
	token1, hash1 := NewToken()
	token2, _ := NewToken()

	token1Bytes, err := base64.RawURLEncoding.DecodeString(token1)
	if err != nil {
		t.Fatalf("token %q is not base64url: %v", token1, err)
	}
	if len(token1Bytes) != 32 {
		t.Errorf("token carries %d random bytes, want 32", len(token1Bytes))
	}
	if token1 == token2 {
		t.Errorf("two calls returned the same token %q", token1)
	}
	if len(hash1) != sha256.Size {
		t.Errorf("hash length = %d bytes, want %d", len(hash1), sha256.Size)
	}
	if !bytes.Equal(HashToken(token1), hash1) {
		t.Errorf("NewToken hash = %x, want HashToken(%q) = %x: the hash in the database will not match the cookie", hash1, token1, HashToken(token1))
	}
}

func TestCookie(t *testing.T) {
	token, _ := NewToken()
	c := setAndParse(t, Cookie(token))

	if c.Name != CookieName {
		t.Errorf("name = %q, want %q", c.Name, CookieName)
	}
	if c.Value != token {
		t.Errorf("value = %q, want token %q: the token did not survive Set-Cookie", c.Value, token)
	}
	if !c.HttpOnly {
		t.Errorf("HttpOnly is not set: scripts on the page can read the token")
	}
	if !c.Secure {
		t.Errorf("Secure is not set: the browser may send the token over plain HTTP")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax (%v)", c.SameSite, http.SameSiteLaxMode)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, want %q", c.Path, "/")
	}
	if want := int(Lifetime.Seconds()); c.MaxAge != want {
		t.Errorf("MaxAge = %d, want %d", c.MaxAge, want)
	}
}

func TestClearCookie(t *testing.T) {
	c := setAndParse(t, ClearCookie())

	if c.Name != CookieName {
		t.Errorf("name = %q, want %q: the browser deletes a cookie only if the name matches", c.Name, CookieName)
	}
	if c.Value != "" {
		t.Errorf("value = %q, want empty", c.Value)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, want %q: the browser deletes a cookie only if the path matches", c.Path, "/")
	}
	if c.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want negative (Max-Age=0 in the header)", c.MaxAge)
	}
}
