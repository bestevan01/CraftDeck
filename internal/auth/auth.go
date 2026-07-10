// Package auth handles password hashing and TOTP-based two-factor
// authentication for the management web UI login (requirements.md FR-32,
// FR-36~39). It intentionally has no knowledge of Minecraft game-port
// access: 2FA in this codebase only ever gates the admin web session.
package auth

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"fmt"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// NewSessionID generates a random, URL/cookie-safe session identifier: 32
// bytes (256 bits) of entropy, hex-encoded so it needs no further escaping
// as a cookie value.
func NewSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// GenerateTOTPSecret creates a new TOTP secret plus a QR-code-ready
// otpauth:// URI for FR-39's enrollment flow.
func GenerateTOTPSecret(username, issuer string) (secret string, otpauthURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate totp secret: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

func ValidateTOTPCode(secret, code string) bool {
	return totp.Validate(code, secret)
}

// GenerateBackupCodes returns n single-use recovery codes for FR-39. Callers
// are responsible for hashing them (e.g. with bcrypt) before storing in
// users.backup_codes_json.
func GenerateBackupCodes(n int) ([]string, error) {
	codes := make([]string, n)
	for i := range codes {
		buf := make([]byte, 5)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("generate backup code: %w", err)
		}
		codes[i] = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	}
	return codes, nil
}
