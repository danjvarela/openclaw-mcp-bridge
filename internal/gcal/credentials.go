// Package gcal implements the Google Calendar tools exposed by the bridge:
// create_event, update_event, delete_event (all restricted to the "Bot"
// calendar) and list_events (any calendar, read-only). Auth is an OAuth2
// refresh-token flow against Google's endpoint; access tokens are minted
// and refreshed automatically by golang.org/x/oauth2, never hand-rolled.
package gcal

import (
	"encoding/json"
	"fmt"
	"os"
)

// CredentialsEnvVar names the env var the bridge reads for the path to the
// credentials JSON file, mirroring the vaultsync VAULT_* convention of
// passing sops-provisioned secret paths via neutral env names rather than
// inlining secret material into env values.
const CredentialsEnvVar = "GOOGLE_CALENDAR_CREDENTIALS"

// Credentials is the {client_id, client_secret, refresh_token} JSON blob
// provisioned by sops as openclaw-google-calendar-credentials (ticket 04).
type Credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// LoadCredentials reads and validates the credentials JSON file at path.
func LoadCredentials(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gcal: read credentials %s: %w", path, err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("gcal: parse credentials %s: %w", path, err)
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
		return nil, fmt.Errorf("gcal: credentials %s missing fields: %v", path, missing)
	}
	return &creds, nil
}

// CredentialsPathFromEnv resolves the credentials file path from
// GOOGLE_CALENDAR_CREDENTIALS, the env var the Nix module wires to the
// sops-provisioned secret path (/run/secrets/openclaw-google-calendar-credentials
// on the VPS).
func CredentialsPathFromEnv(getenv func(string) string) (string, error) {
	path := getenv(CredentialsEnvVar)
	if path == "" {
		return "", fmt.Errorf("gcal: missing required env: %s", CredentialsEnvVar)
	}
	return path, nil
}
