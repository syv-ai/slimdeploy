package api

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

const (
	sessionCookieName = "slimdeploy_session"
	sessionDuration   = 7 * 24 * time.Hour // 7 days
)

// AuthManager handles authentication
type AuthManager struct {
	db       *sql.DB
	password string
}

// NewAuthManager creates a new auth manager
func NewAuthManager(db *sql.DB, password string) *AuthManager {
	return &AuthManager{
		db:       db,
		password: password,
	}
}

// ValidatePassword checks if the provided password is correct
func (am *AuthManager) ValidatePassword(password string) bool {
	return subtle.ConstantTimeCompare([]byte(am.password), []byte(password)) == 1
}

// CreateSession creates a new session and returns the token
func (am *AuthManager) CreateSession() (string, error) {
	// Generate random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Store session
	expiresAt := time.Now().Add(sessionDuration)
	_, err := am.db.Exec(
		"INSERT INTO sessions (token, expires_at) VALUES (?, ?)",
		token, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	return token, nil
}

// ValidateSession checks if a session token is valid
func (am *AuthManager) ValidateSession(token string) bool {
	if token == "" {
		return false
	}

	var expiresAt time.Time
	err := am.db.QueryRow(
		"SELECT expires_at FROM sessions WHERE token = ?",
		token,
	).Scan(&expiresAt)

	if err != nil {
		return false
	}

	if time.Now().After(expiresAt) {
		// Session expired, delete it
		am.DeleteSession(token)
		return false
	}

	return true
}

// DeleteSession deletes a session
func (am *AuthManager) DeleteSession(token string) error {
	_, err := am.db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

// CleanupExpiredSessions removes expired sessions
func (am *AuthManager) CleanupExpiredSessions() error {
	_, err := am.db.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}

// SetSessionCookie sets the session cookie on the response
func (am *AuthManager) SetSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	// Only set Secure flag if request came over HTTPS
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie clears the session cookie
func (am *AuthManager) ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// GetSessionFromRequest gets the session token from the request
func (am *AuthManager) GetSessionFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// IsAuthenticated checks if the request is authenticated
func (am *AuthManager) IsAuthenticated(r *http.Request) bool {
	token := am.GetSessionFromRequest(r)
	return am.ValidateSession(token)
}
