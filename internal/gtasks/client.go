package gtasks

import (
	"context"
	"net/http"

	"github.com/danjvarela/openclaw-mcp-bridge/internal/googleoauth"
)

// NewHTTPClient returns an *http.Client that attaches a Google OAuth2
// bearer token to every request. See googleoauth.NewHTTPClient.
func NewHTTPClient(ctx context.Context, creds *Credentials) *http.Client {
	return googleoauth.NewHTTPClient(ctx, creds)
}
