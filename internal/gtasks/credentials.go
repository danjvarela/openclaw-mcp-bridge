package gtasks

import "github.com/danjvarela/openclaw-mcp-bridge/internal/googleoauth"

// CredentialsEnvVar names the env var the bridge reads for the path to the
// credentials JSON file, shared with gcal: a single refresh token
// consented with the union of Calendar and Tasks scopes covers both. The
// refresh_token must include the full https://www.googleapis.com/auth/tasks
// scope (not tasks.readonly): it permits tasklists.insert, which
// EnsureTaskList relies on to auto-create the Bot list.
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
