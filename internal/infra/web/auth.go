package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ===== Session/JWT primitives =====

type AuthConfig struct {
	HMACSecret   []byte
	CookieName   string
	CookieDomain string
	SecureCookie bool
	TTL          time.Duration
}

type AuthManager struct{ cfg AuthConfig }

func NewAuthManager(secret string, secure bool, domain string, ttl time.Duration) *AuthManager {
	return &AuthManager{cfg: AuthConfig{
		HMACSecret:   []byte(secret),
		CookieName:   "admin_session",
		CookieDomain: domain, // "" is fine if you want host-only cookie
		SecureCookie: secure, // true in prod (TLS)
		TTL:          ttl,    // e.g., 30 * time.Minute
	}}
}

type AdminClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func (a *AuthManager) Mint(w http.ResponseWriter) (string, error) {
	now := time.Now()
	claims := AdminClaims{
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(a.cfg.TTL)),
			Subject:   "admin",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(a.cfg.HMACSecret)
	if err != nil {
		return "", err
	}

	c := &http.Cookie{
		Name:     a.cfg.CookieName,
		Value:    signed,
		Path:     "/",
		Domain:   a.cfg.CookieDomain,
		MaxAge:   int(a.cfg.TTL.Seconds()),
		HttpOnly: true,
		Secure:   a.cfg.SecureCookie,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, c)
	return signed, nil
}

func (a *AuthManager) Clear(w http.ResponseWriter) {
	c := &http.Cookie{
		Name:     a.cfg.CookieName,
		Value:    "",
		Path:     "/",
		Domain:   a.cfg.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cfg.SecureCookie,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, c)
}

func (a *AuthManager) ParseFromRequest(r *http.Request) (*AdminClaims, error) {
	// Authorization: Bearer <jwt>
	if hdr := r.Header.Get("Authorization"); hdr != "" {
		if strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
			return a.parse(strings.TrimSpace(hdr[7:]))
		}
	}
	// Cookie
	if c, err := r.Cookie(a.cfg.CookieName); err == nil {
		return a.parse(c.Value)
	}
	return nil, errors.New("missing token")
}

func (a *AuthManager) parse(tok string) (*AdminClaims, error) {
	claims := &AdminClaims{}
	tkn, err := jwt.ParseWithClaims(tok, claims, func(t *jwt.Token) (any, error) {
		return a.cfg.HMACSecret, nil
	})
	if err != nil || !tkn.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
