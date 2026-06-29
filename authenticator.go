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
	"sync/atomic"
	"time"

	"github.com/go-jose/go-jose/v4"
)

type Authenticator struct {
	Host         string
	ClientID     string
	ClientSecret string

	tk        atomic.Pointer[jwtToken]
	publicKey *rsa.PublicKey
	client    *http.Client

	refreshMu sync.Mutex
}
type jwtToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpireIn     int    `json:"expires_in"`

	expiresAt time.Time
}

const tokenExpiryLeeway = 10 * time.Second

func (tk *jwtToken) expired() bool {
	if tk.expiresAt.IsZero() {
		return false
	}
	return time.Until(tk.expiresAt) <= tokenExpiryLeeway
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

	tk, err := t.token(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	t.tk.Store(tk)

	ticker := time.NewTicker(t.refreshInterval())

	go func() {
		defer ticker.Stop()
	loop:
		for {
			select {
			case <-ticker.C:
				if err := t.refresh(ctx, t.tk.Load()); err != nil {
					log.Printf("[ERROR] failed to refresh token: %v", err)
					ticker.Reset(time.Minute)
					continue
				}
				ticker.Reset(t.refreshInterval())
			case <-ctx.Done():
				break loop
			}
		}
	}()

	return t, nil
}

// Token returns the current access token. If no token is held or the current one
// has expired (e.g. the background refresher stalled or its context was
// canceled), it refreshes on demand before returning, so freshness does not
// depend on the background goroutine being alive. If that refresh fails it falls
// back to the existing token rather than an empty string.
func (t *Authenticator) Token() string {
	tk := t.tk.Load()

	if tk == nil || tk.expired() {
		if err := t.refresh(context.Background(), tk); err != nil {
			log.Printf("[ERROR] failed to refresh token: %v", err)
		}
		tk = t.tk.Load()
	}

	if tk == nil {
		return ""
	}

	return tk.AccessToken
}

// Refresh forces a token refresh and reports whether a new token was obtained.
// Call it when a downstream request made with Token() comes back 401
// (Unauthorized), then retry the request with the refreshed Token().
func (t *Authenticator) Refresh(ctx context.Context) bool {
	if err := t.refresh(ctx, t.tk.Load()); err != nil {
		log.Printf("[ERROR] failed to refresh token: %v", err)
		return false
	}
	return true
}

// Verify validates the signature of the given JWT against the auth service's public key
// and returns the decoded payload.
func (t *Authenticator) Verify(token string) ([]byte, error) {
	jws, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	// publicKey is immutable after construction, so no synchronisation is needed.
	payload, err := jws.Verify(t.publicKey)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	return payload, nil
}

// token requests a token from the auth service. When cur carries a refresh token
// the refresh_token grant is used; otherwise it performs a client_credentials grant.
func (t *Authenticator) token(ctx context.Context, cur *jwtToken) (*jwtToken, error) {
	data := url.Values{}

	if cur == nil || cur.RefreshToken == "" {
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

	if token.ExpireIn > 0 {
		token.expiresAt = time.Now().Add(time.Duration(token.ExpireIn) * time.Second)
	}

	return token, nil
}

// refresh obtains a new token and atomically swaps it in. It first tries the
// refresh_token grant and, on failure, falls back to a fresh client_credentials
// grant. The currently stored token is left untouched until a new one is
// successfully fetched, so Token() never observes an empty value.
//
// old is the token the caller observed before deciding to refresh. If another
// goroutine already replaced it while this one waited for refreshMu, the refresh
// is skipped — collapsing a burst of concurrent 401s into a single network call.
func (t *Authenticator) refresh(ctx context.Context, old *jwtToken) error {
	t.refreshMu.Lock()
	defer t.refreshMu.Unlock()

	cur := t.tk.Load()
	if old != nil && cur != old {
		return nil
	}

	tk, err := t.token(ctx, cur)
	if err != nil {
		// The refresh token may be expired/revoked; re-authenticate from scratch.
		tk, err = t.token(ctx, nil)
		if err != nil {
			return err
		}
	}

	t.tk.Store(tk)

	return nil
}

// refreshInterval returns how long to wait before refreshing the current token,
// scheduling the refresh at ~80% of the token's lifetime so the margin scales
// with short-lived tokens instead of a fixed offset that could exceed the TTL.
func (t *Authenticator) refreshInterval() time.Duration {
	tk := t.tk.Load()

	if tk == nil || tk.ExpireIn <= 0 {
		return time.Minute
	}

	d := time.Duration(tk.ExpireIn) * time.Second

	return d - d/5
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
