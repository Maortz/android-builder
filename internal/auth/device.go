package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	clientID       = "Ov23licGIxOtKeO06DDK"
	deviceCodeURL  = "https://github.com/login/device/code"
	accessTokenURL = "https://github.com/login/oauth/access_token"
	scope          = "repo workflow"
)

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

func DeviceLogin(ctx context.Context) (string, error) {
	resp, err := jsonPost(deviceCodeURL, url.Values{
		"client_id": {clientID},
		"scope":     {scope},
	})
	if err != nil {
		return "", fmt.Errorf("request device code: %w", err)
	}
	defer resp.Body.Close()

	var dc deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		return "", fmt.Errorf("decode device code: %w", err)
	}

	fmt.Printf("\nAuthenticating with GitHub...\n\n")
	fmt.Printf("  Open: %s\n", dc.VerificationURI)
	fmt.Printf("  Enter code: %s\n\n", dc.UserCode)

	interval := time.Duration(dc.Interval) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}

		resp, err := jsonPost(accessTokenURL, url.Values{
			"client_id":   {clientID},
			"device_code": {dc.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		})
		if err != nil {
			continue
		}

		var at accessTokenResponse
		json.NewDecoder(resp.Body).Decode(&at)
		resp.Body.Close()

		switch at.Error {
		case "":
			if at.AccessToken != "" {
				return at.AccessToken, nil
			}
		case "authorization_pending":
			// keep polling
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return "", fmt.Errorf("code expired — run again")
		case "access_denied":
			return "", fmt.Errorf("access denied")
		default:
			return "", fmt.Errorf("auth error: %s", at.Error)
		}
	}
	return "", fmt.Errorf("timed out waiting for authorization")
}

func jsonPost(endpoint string, values url.Values) (*http.Response, error) {
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return http.DefaultClient.Do(req)
}
