// Package gcal implements the Google Calendar tools exposed by the bridge:
// create_event, update_event, delete_event (all restricted to the "Bot"
// calendar) and list_events (any calendar, read-only). Auth is an OAuth2
// refresh-token flow against Google's endpoint; access tokens are minted
// and refreshed automatically by golang.org/x/oauth2, never hand-rolled.
package gcal

import "github.com/danjvarela/openclaw-mcp-bridge/internal/googleoauth"

// CredentialsEnvVar names the env var the bridge reads for the path to the
// credentials JSON file, shared with gtasks: a single refresh token
// consented with the union of Calendar and Tasks scopes covers both.
const CredentialsEnvVar = googleoauth.CredentialsEnvVar

// Credentials is the {client_id, client_secret, refresh_token} JSON blob
// provisioned by sops as openclaw-google-credentials.
type Credentials = googleoauth.Credentials

// LoadCredentials reads and validates the credentials JSON file at path.
func LoadCredentials(path string) (*Credentials, error) {
	return googleoauth.LoadCredentials(path)
}

// CredentialsPathFromEnv resolves the credentials file path from
// GOOGLE_CREDENTIALS, the env var the Nix module wires to the
// sops-provisioned secret path (/run/secrets/openclaw-google-credentials
// on the VPS).
func CredentialsPathFromEnv(getenv func(string) string) (string, error) {
	return googleoauth.PathFromEnv(CredentialsEnvVar, getenv)
}
