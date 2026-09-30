package gtasks

import "github.com/danjvarela/openclaw-mcp-bridge/internal/googleoauth"

// CredentialsEnvVar names the env var the bridge reads for the path to the
// credentials JSON file. Deliberately distinct from gcal's
// GOOGLE_CALENDAR_CREDENTIALS: this package's refresh token was minted with
// the `tasks` scope, which Calendar's token was never consented for.
const CredentialsEnvVar = "GOOGLE_TASKS_CREDENTIALS"

// Credentials is the {client_id, client_secret, refresh_token} JSON blob
// provisioned by sops as openclaw-google-tasks-credentials. The
// refresh_token must be consented with the full
// https://www.googleapis.com/auth/tasks scope (not tasks.readonly): unlike
// Calendar's split scopes, this scope does permit tasklists.insert, which
// EnsureTaskList relies on to auto-create the Bot list.
type Credentials = googleoauth.Credentials

// LoadCredentials reads and validates the credentials JSON file at path.
func LoadCredentials(path string) (*Credentials, error) {
	return googleoauth.LoadCredentials(path)
}

// CredentialsPathFromEnv resolves the credentials file path from
// GOOGLE_TASKS_CREDENTIALS, the env var the Nix module wires to the
// sops-provisioned secret path (/run/secrets/openclaw-google-tasks-credentials
// on the VPS).
func CredentialsPathFromEnv(getenv func(string) string) (string, error) {
	return googleoauth.PathFromEnv(CredentialsEnvVar, getenv)
}
