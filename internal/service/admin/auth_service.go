package admin

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
	"synova-rd-workflow/internal/domain"
)

const tokenTTL = 8 * time.Hour

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrWeakPassword       = errors.New("weak password")
	ErrCurrentPassword    = errors.New("current password does not match")
)

type ConfigStore interface {
	GetConfig(ctx context.Context) (domain.AdminConfig, error)
	SaveConfig(ctx context.Context, cfg domain.AdminConfig) error
}

type AuthService struct {
	store       ConfigStore
	email       string
	initialHash string
	privateKey  *rsa.PrivateKey
	publicKey   *rsa.PublicKey
}

type Claims struct {
	Email              string `json:"email"`
	MustChangePassword bool   `json:"must_change_password"`
	ExpiresAt          int64  `json:"exp"`
	IssuedAt           int64  `json:"iat"`
}

func NewAuthService(store ConfigStore, email, initialHash, privatePEM, publicPEM string) (*AuthService, error) {
	privateKey, publicKey, err := loadOrGenerateKeys(privatePEM, publicPEM)
	if err != nil {
		return nil, err
	}
	return &AuthService{
		store:       store,
		email:       strings.ToLower(strings.TrimSpace(email)),
		initialHash: initialHash,
		privateKey:  privateKey,
		publicKey:   publicKey,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, domain.AdminConfig, error) {
	cfg, err := s.currentConfig(ctx)
	if err != nil {
		return "", domain.AdminConfig{}, err
	}
	if strings.ToLower(strings.TrimSpace(email)) != strings.ToLower(cfg.Email) {
		return "", domain.AdminConfig{}, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(password)) != nil {
		return "", domain.AdminConfig{}, ErrInvalidCredentials
	}
	token, err := s.Sign(Claims{
		Email:              cfg.Email,
		MustChangePassword: cfg.MustChangePassword,
		IssuedAt:           time.Now().Unix(),
		ExpiresAt:          time.Now().Add(tokenTTL).Unix(),
	})
	if err != nil {
		return "", domain.AdminConfig{}, err
	}
	return token, cfg, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, currentPassword, newPassword string) (string, domain.AdminConfig, error) {
	cfg, err := s.currentConfig(ctx)
	if err != nil {
		return "", domain.AdminConfig{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(currentPassword)) != nil {
		return "", domain.AdminConfig{}, ErrCurrentPassword
	}
	if !strongPassword(newPassword) {
		return "", domain.AdminConfig{}, ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return "", domain.AdminConfig{}, fmt.Errorf("bcrypt generate: %w", err)
	}
	cfg.PasswordHash = string(hash)
	cfg.MustChangePassword = false
	if err := s.store.SaveConfig(ctx, cfg); err != nil {
		return "", domain.AdminConfig{}, err
	}
	token, err := s.Sign(Claims{
		Email:              cfg.Email,
		MustChangePassword: false,
		IssuedAt:           time.Now().Unix(),
		ExpiresAt:          time.Now().Add(tokenTTL).Unix(),
	})
	if err != nil {
		return "", domain.AdminConfig{}, err
	}
	return token, cfg, nil
}

func (s *AuthService) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidCredentials
	}
	signed := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, err
	}
	digest := sha256.Sum256([]byte(signed))
	if err := rsa.VerifyPKCS1v15(s.publicKey, crypto.SHA256, digest[:], sig); err != nil {
		return Claims{}, ErrInvalidCredentials
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, err
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, err
	}
	if claims.ExpiresAt < time.Now().Unix() {
		return Claims{}, ErrInvalidCredentials
	}
	return claims, nil
}

func (s *AuthService) Sign(claims Claims) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signed := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *AuthService) currentConfig(ctx context.Context) (domain.AdminConfig, error) {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return domain.AdminConfig{}, err
	}
	if cfg.Email != "" {
		return cfg, nil
	}
	hash := s.initialHash
	if hash == "" {
		generated, err := bcrypt.GenerateFromPassword([]byte("ChangeMe#2026"), 12)
		if err != nil {
			return domain.AdminConfig{}, err
		}
		hash = string(generated)
	}
	cfg = domain.AdminConfig{
		Email:              s.email,
		PasswordHash:       hash,
		MustChangePassword: true,
	}
	if err := s.store.SaveConfig(ctx, cfg); err != nil {
		return domain.AdminConfig{}, err
	}
	return cfg, nil
}

func strongPassword(v string) bool {
	if len(v) < 12 {
		return false
	}
	var upper, digit, symbol bool
	for _, r := range v {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			symbol = true
		}
	}
	return upper && digit && symbol
}

func loadOrGenerateKeys(privateRaw, publicRaw string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	if privateRaw == "" {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	}
	privatePEM, err := decodeMaybeBase64(privateRaw)
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(privatePEM)
	if block == nil {
		return nil, nil, errors.New("invalid ADMIN_JWT_PRIVATE_KEY pem")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsed, parseErr := x509.ParsePKCS8PrivateKey(block.Bytes)
		if parseErr != nil {
			return nil, nil, err
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, errors.New("ADMIN_JWT_PRIVATE_KEY is not RSA")
		}
		key = rsaKey
	}
	if publicRaw == "" {
		return key, &key.PublicKey, nil
	}
	publicPEM, err := decodeMaybeBase64(publicRaw)
	if err != nil {
		return nil, nil, err
	}
	pubBlock, _ := pem.Decode(publicPEM)
	if pubBlock == nil {
		return nil, nil, errors.New("invalid ADMIN_JWT_PUBLIC_KEY pem")
	}
	parsed, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	pub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, nil, errors.New("ADMIN_JWT_PUBLIC_KEY is not RSA")
	}
	return key, pub, nil
}

func decodeMaybeBase64(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "BEGIN ") {
		return []byte(raw), nil
	}
	return base64.StdEncoding.DecodeString(raw)
}
