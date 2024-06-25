package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type Authenticator struct {
	Host         string
	ClientID     string
	ClientSecret string

	tk        *jwtToken
	publicKey *rsa.PublicKey

	sync.RWMutex
}
type jwtToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpireIn     int    `json:"expires_in"`
}

type JSONWebKey struct {
	Key       interface{}
	KeyID     string
	Algorithm string
	Use       string

	Certificates                []*x509.Certificate
	CertificatesURL             *url.URL
	CertificateThumbprintSHA1   []byte
	CertificateThumbprintSHA256 []byte
}
type JSONWebKeySet struct {
	Keys []JSONWebKey `json:"keys"`
}

func (k *JSONWebKey) IsPublic() bool {
	switch k.Key.(type) {
	case *ecdsa.PublicKey, *rsa.PublicKey, ed25519.PublicKey:
		return true
	default:
		return false
	}
}

func NewAuthenticator(ctx context.Context) (*Authenticator, error) {
	t := &Authenticator{
		Host:         os.Getenv("AUTH_HOST"),
		ClientID:     os.Getenv("AUTH_CLIENT_ID"),
		ClientSecret: os.Getenv("AUTH_CLIENT_SECRET"),
	}
	if t.Host == "" {
		return nil, fmt.Errorf("AUTH_HOST not found")
	}
	if t.ClientID == "" {
		return nil, fmt.Errorf("AUTH_CLIENT_ID not found")
	}
	if t.ClientSecret == "" {
		return nil, fmt.Errorf("AUTH_CLIENT_SECRET not found")
	}

	publicKey, err := getPublicKey(ctx, t.Host)
	if err != nil {
		return nil, err
	}
	t.publicKey = publicKey

	t.tk, err = t.token(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %v", err)
	}

	ticker := time.NewTicker(time.Duration(t.tk.ExpireIn)*time.Second - time.Minute)
	go func() {
	loop:
		for {
			select {
			case <-ticker.C:
				tk, err := t.token(ctx)
				if err != nil {
					fmt.Printf("failed to get token: %v\n", err)

					t.Lock()
					t.tk = nil
					t.Unlock()

					tk, err = t.token(ctx)
					if err != nil {
						fmt.Printf("failed to get token: %v\n", err)
					}
				}

				t.Lock()
				t.tk = tk
				t.Unlock()

				ticker.Reset(time.Duration(t.tk.ExpireIn)*time.Second - time.Minute)
			case <-ctx.Done():
				break loop
			}
		}
	}()

	return t, nil
}

func (t *Authenticator) Token() string {
	t.RLock()
	defer t.RUnlock()
	return t.tk.AccessToken
}
func (t *Authenticator) token(ctx context.Context) (*jwtToken, error) {
	data := url.Values{}

	if t.tk == nil {
		data.Set("grant_type", "client_credentials")
		data.Set("client_id", t.ClientID)
		data.Set("client_secret", t.ClientSecret)
	} else {
		data.Set("grant_type", "refresh_token")
		data.Set("refresh_token", t.tk.RefreshToken)
	}

	uri := fmt.Sprintf("%s/token", t.Host)
	req, err := http.NewRequest("POST", uri, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request to %s: %w", uri, err)
	}

	req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := http.Client{
		Timeout: time.Minute,
	}
	resp, err := client.Do(req)
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
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/.well-known/jwks.json", host), nil)
	req.WithContext(ctx)

	client := http.Client{
		Timeout: time.Second * 3,
	}

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("wrong status code: %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jwks = JSONWebKeySet{}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, err
	}

	publicJWKS := jwks.Keys
	if len(publicJWKS) == 0 {
		return nil, errors.New("public JWKS not found")
	}

	publicJWK := publicJWKS[0]
	if !publicJWK.IsPublic() {
		return nil, errors.New("JWK is not public key")
	}

	return publicJWK.Key.(*rsa.PublicKey), nil
}
