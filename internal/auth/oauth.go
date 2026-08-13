package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrOAuthExpired = errors.New("GitHub authorization expired")
	ErrOAuthDenied  = errors.New("GitHub authorization was denied")
)

type DeviceStart struct {
	UserCode        string        `json:"userCode"`
	VerificationURI string        `json:"verificationUri"`
	ExpiresIn       time.Duration `json:"expiresIn"`
}

type OAuthClient struct {
	ClientID string
	HTTP     *http.Client
	BaseURL  string
	APIURL   string
	Sleep    func(context.Context, time.Duration) error

	mu         sync.Mutex
	deviceCode string
	interval   time.Duration
	deadline   time.Time
}

func NewOAuthClient(clientID string, client *http.Client) *OAuthClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &OAuthClient{
		ClientID: clientID,
		HTTP:     client,
		BaseURL:  "https://github.com",
		APIURL:   "https://api.github.com",
		Sleep:    sleepContext,
	}
}

func (c *OAuthClient) Start(ctx context.Context) (DeviceStart, error) {
	if c.ClientID == "" {
		return DeviceStart{}, errors.New("GitHub OAuth client ID is not configured")
	}
	values := url.Values{
		"client_id": {c.ClientID},
		"scope":     {"repo"},
	}
	var response struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := c.postForm(ctx, c.BaseURL+"/login/device/code", values, &response); err != nil {
		return DeviceStart{}, err
	}
	if response.DeviceCode == "" || response.UserCode == "" ||
		response.VerificationURI != "https://github.com/login/device" ||
		response.ExpiresIn <= 0 {
		return DeviceStart{}, errors.New("GitHub returned an invalid device authorization response")
	}
	interval := time.Duration(response.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	expiresIn := time.Duration(response.ExpiresIn) * time.Second

	c.mu.Lock()
	c.deviceCode = response.DeviceCode
	c.interval = interval
	c.deadline = time.Now().Add(expiresIn)
	c.mu.Unlock()

	return DeviceStart{
		UserCode:        response.UserCode,
		VerificationURI: response.VerificationURI,
		ExpiresIn:       expiresIn,
	}, nil
}

func (c *OAuthClient) Wait(ctx context.Context) (Account, string, error) {
	c.mu.Lock()
	deviceCode := c.deviceCode
	interval := c.interval
	deadline := c.deadline
	c.mu.Unlock()
	if deviceCode == "" {
		return Account{}, "", errors.New("device authorization has not been started")
	}

	for {
		if !time.Now().Before(deadline) {
			c.clearDeviceCode()
			return Account{}, "", ErrOAuthExpired
		}
		if err := c.Sleep(ctx, interval); err != nil {
			return Account{}, "", err
		}
		token, scopes, oauthError, err := c.poll(ctx, deviceCode)
		if err != nil {
			return Account{}, "", err
		}
		switch oauthError {
		case "":
			if !containsScope(scopes, "repo") {
				c.clearDeviceCode()
				return Account{}, "", errors.New("GitHub authorization did not grant repo scope")
			}
			account, err := c.fetchAccount(ctx, token, scopes)
			if err != nil {
				return Account{}, "", err
			}
			c.clearDeviceCode()
			return account, token, nil
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			c.clearDeviceCode()
			return Account{}, "", ErrOAuthExpired
		case "access_denied":
			c.clearDeviceCode()
			return Account{}, "", ErrOAuthDenied
		default:
			c.clearDeviceCode()
			return Account{}, "", fmt.Errorf("GitHub authorization failed: %s", oauthError)
		}
	}
}

func (c *OAuthClient) poll(ctx context.Context, deviceCode string) (string, []string, string, error) {
	values := url.Values{
		"client_id":   {c.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	var response struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	if err := c.postForm(ctx, c.BaseURL+"/login/oauth/access_token", values, &response); err != nil {
		return "", nil, "", err
	}
	if response.Error == "" && (response.AccessToken == "" || !strings.EqualFold(response.TokenType, "bearer")) {
		return "", nil, "", errors.New("GitHub returned an invalid access token response")
	}
	return response.AccessToken, splitScopes(response.Scope), response.Error, nil
}

func (c *OAuthClient) fetchAccount(ctx context.Context, token string, scopes []string) (Account, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIURL+"/user", nil)
	if err != nil {
		return Account{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	var account Account
	if err := c.doJSON(request, &account); err != nil {
		return Account{}, err
	}
	if account.ID == 0 || account.Login == "" {
		return Account{}, errors.New("GitHub returned an invalid user profile")
	}
	account.Scopes = scopes
	return account, nil
}

func (c *OAuthClient) postForm(ctx context.Context, endpoint string, values url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.doJSON(request, target)
}

func (c *OAuthClient) doJSON(request *http.Request, target any) error {
	response, err := c.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("GitHub request failed with status %s", response.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func (c *OAuthClient) clearDeviceCode() {
	c.mu.Lock()
	c.deviceCode = ""
	c.deadline = time.Time{}
	c.mu.Unlock()
}

func (c *OAuthClient) Cancel() {
	c.clearDeviceCode()
}

func splitScopes(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	})
}

func containsScope(scopes []string, wanted string) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
