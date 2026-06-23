package service

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
)

type Authenticator struct {
	Host         string
	ClientID     string
	ClientSecret string

	tk        *jwtToken
	publicKey *rsa.PublicKey
	client    *http.Client

	sync.RWMutex
}
type jwtToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpireIn     int    `json:"expires_in"`
}

var (
	ErrAuthHostNotFound         = errors.New("AUTH_HOST not found")
	ErrAuthClientIDNotFound     = errors.New("AUTH_CLIENT_ID not found")
	ErrAuthClientSecretNotFound = errors.New("AUTH_CLIENT_SECRET not found")
)

func NewAuthenticator(ctx context.Context) (*Authenticator, error) {
	t := &Authenticator{
		Host:         os.Getenv("AUTH_HOST"),
		ClientID:     os.Getenv("AUTH_CLIENT_ID"),
		ClientSecret: os.Getenv("AUTH_CLIENT_SECRET"),
		client:       &http.Client{Timeout: time.Minute},
	}

	if t.Host == "" {
		return nil, ErrAuthHostNotFound
	}
	if t.ClientID == "" {
		return nil, ErrAuthClientIDNotFound
	}
	if t.ClientSecret == "" {
		return nil, ErrAuthClientSecretNotFound
	}

	publicKey, err := getPublicKey(ctx, t.Host)
	if err != nil {
		return nil, err
	}
	t.publicKey = publicKey

	t.tk, err = t.token(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	ticker := time.NewTicker(t.refreshInterval())
	defer ticker.Stop()

	go func() {
	loop:
		for {
			select {
			case <-ticker.C:
				tk, err := t.token(ctx)
				if err != nil {
					t.Lock()
					t.tk = nil
					t.Unlock()

					tk, err = t.token(ctx)
					if err != nil {
						log.Printf("[ERROR] failed to refresh token: %v", err)
						ticker.Reset(time.Minute)
						continue
					}
				}

				t.Lock()
				t.tk = tk
				t.Unlock()

				ticker.Reset(t.refreshInterval())
			case <-ctx.Done():
				break loop
			}
		}
	}()

	return t, nil
}

// refreshInterval returns how long to wait before refreshing the current token,
// leaving a one-minute safety margin before expiry.
func (t *Authenticator) refreshInterval() time.Duration {
	t.RLock()
	defer t.RUnlock()

	if t.tk == nil || t.tk.ExpireIn <= 0 {
		return time.Minute
	}
	if d := time.Duration(t.tk.ExpireIn)*time.Second - time.Minute; d > 0 {
		return d
	}

	return time.Minute
}

// Token returns the current access token, or an empty string if none is available.
func (t *Authenticator) Token() string {
	t.RLock()
	defer t.RUnlock()
	if t.tk == nil {
		return ""
	}
	return t.tk.AccessToken
}

// Verify validates the signature of the given JWT against the auth service's public key
// and returns the decoded payload.
func (t *Authenticator) Verify(token string) ([]byte, error) {
	jws, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	t.RLock()
	key := t.publicKey
	t.RUnlock()

	payload, err := jws.Verify(key)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	return payload, nil
}

func (t *Authenticator) token(ctx context.Context) (*jwtToken, error) {
	data := url.Values{}

	t.RLock()
	cur := t.tk
	t.RUnlock()

	if cur == nil {
		data.Set("grant_type", "client_credentials")
		data.Set("client_id", t.ClientID)
		data.Set("client_secret", t.ClientSecret)
	} else {
		data.Set("grant_type", "refresh_token")
		data.Set("refresh_token", cur.RefreshToken)
	}

	uri := fmt.Sprintf("%s/token", t.Host)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request to %s: %w", uri, err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%d: %s", resp.StatusCode, string(body))
	}

	token := &jwtToken{}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("failed to decode response from auth service: %w", err)
	}

	return token, nil
}

func getPublicKey(ctx context.Context, host string) (*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/.well-known/jwks.json", host), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := http.Client{
		Timeout: time.Second * 3,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wrong status code: %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jwks = jose.JSONWebKeySet{}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, err
	}

	publicJWKS := jwks.Keys
	if len(publicJWKS) == 0 {
		return nil, errors.New("public JWKS not found")
	}

	publicJWK := publicJWKS[0]

	if keyID := os.Getenv("JWKS_KEY_ID"); keyID != "" {
		found := false

		for _, key := range publicJWKS {
			if key.KeyID == keyID {
				publicJWK = key
				found = true
				break
			}
		}

		if !found {
			return nil, errors.New("public JWK not found")
		}
	}

	if !publicJWK.IsPublic() {
		return nil, errors.New("JWK is not public key")
	}

	key, ok := publicJWK.Key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("JWK is not an RSA public key")
	}

	return key, nil
}
