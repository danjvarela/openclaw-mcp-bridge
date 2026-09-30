// Package googleoauth is the shared OAuth2 refresh-token plumbing used by
// every Google API this bridge talks to (Calendar, Tasks, ...). Each API
// package keeps its own env var name and credentials file convention but
// delegates token loading and HTTP client construction here to avoid
// duplicating the oauth2.Config wiring per package.
package googleoauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Credentials is the {client_id, client_secret, refresh_token} JSON blob
// provisioned by sops for a given Google API integration.
type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// LoadCredentials reads and validates the credentials JSON file at path.
func LoadCredentials(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("googleoauth: read credentials %s: %w", path, err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("googleoauth: parse credentials %s: %w", path, err)
	}
	var missing []string
	for _, kv := range [][2]string{
		{"client_id", creds.ClientID},
		{"client_secret", creds.ClientSecret},
		{"refresh_token", creds.RefreshToken},
	} {
		if kv[1] == "" {
			missing = append(missing, kv[0])
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("googleoauth: credentials %s missing fields: %v", path, missing)
	}
	return &creds, nil
}

// PathFromEnv resolves the credentials file path from envVar, the env var a
// Nix module wires to a sops-provisioned secret path.
func PathFromEnv(envVar string, getenv func(string) string) (string, error) {
	path := getenv(envVar)
	if path == "" {
		return "", fmt.Errorf("googleoauth: missing required env: %s", envVar)
	}
	return path, nil
}

// NewHTTPClient returns an *http.Client that attaches a Google OAuth2
// bearer token to every request, automatically minting and refreshing
// access tokens from creds.RefreshToken via oauth2.Config.Client / a
// TokenSource. No refresh logic is hand-rolled here.
func NewHTTPClient(ctx context.Context, creds *Credentials) *http.Client {
	conf := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     google.Endpoint,
	}
	tok := &oauth2.Token{RefreshToken: creds.RefreshToken}
	return conf.Client(ctx, tok)
}
