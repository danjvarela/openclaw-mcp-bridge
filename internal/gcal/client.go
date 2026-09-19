package gcal

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

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
