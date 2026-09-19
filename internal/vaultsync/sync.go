// Package vaultsync runs the vault git write cycle under an exclusive file
// lock. It is the host-side implementation behind the sync_notes MCP tool:
// pull --rebase -> add inbox/ -> commit-if-changed -> push.
//
// The bridge process is spawned by the OpenClaw gateway and runs as the
// openclaw user, so it can read the sops-provisioned deploy key and known_hosts
// directly from /run/secrets. OpenClaw strips GIT_SSH_COMMAND and HOME from a
// server's configured env, so the caller passes the pieces (key path, known
// hosts path, git identity) via neutral VAULT_* env names and this package
// rebuilds GIT_SSH_COMMAND and HOME on the git child process itself.
package vaultsync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Config is the sync cycle configuration, sourced from VAULT_* env.
type Config struct {
	Dir         string // vault repo checkout (cwd for git)
	DeployKey   string // ssh private key path
	KnownHosts  string // ssh known_hosts path
	AuthorName  string // git commit author name
	AuthorEmail string // git commit author email
	GitBin      string // git binary path (default "git")
	SSHBin      string // ssh binary path (default "ssh")
	LockPath    string // shared lock file path

	// LockTimeout bounds how long Run waits to acquire the lock before
	// giving up. Zero means a 60s default. The vault-sync timer holds the
	// same lock for a brief pull, so a bounded wait avoids a tool hang.
	LockTimeout time.Duration
}

// Result summarizes one sync cycle for the tool's text response.
type Result struct {
	Pulled        bool   // a pull --rebase ran
	Committed     bool   // a commit was created
	Pushed        bool   // a push ran (attempted after a commit)
	CommitSubject string // the commit subject, if Committed
}

func (r *Result) String() string {
	if r == nil {
		return "sync ran"
	}
	parts := []string{"pulled"}
	if r.Committed {
		parts = append(parts, "committed: "+r.CommitSubject)
	} else {
		parts = append(parts, "no uncommitted inbox changes")
	}
	if r.Pushed {
		parts = append(parts, "pushed")
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}

// FromEnv builds Config from VAULT_* env vars. Required: Dir, DeployKey,
// KnownHosts, LockPath. GitBin/SSHBin default to PATH lookup.
func FromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		Dir:         getenv("VAULT_DIR"),
		DeployKey:   getenv("VAULT_DEPLOY_KEY"),
		KnownHosts:  getenv("VAULT_KNOWN_HOSTS"),
		AuthorName:  getenv("VAULT_AUTHOR_NAME"),
		AuthorEmail: getenv("VAULT_AUTHOR_EMAIL"),
		GitBin:      getenv("VAULT_GIT_BIN"),
		SSHBin:      getenv("VAULT_SSH_BIN"),
		LockPath:    getenv("VAULT_LOCK"),
	}
	if cfg.GitBin == "" {
		cfg.GitBin = "git"
	}
	if cfg.SSHBin == "" {
		cfg.SSHBin = "ssh"
	}
	var missing []string
	for _, kv := range [][2]string{
		{"VAULT_DIR", cfg.Dir},
		{"VAULT_DEPLOY_KEY", cfg.DeployKey},
		{"VAULT_KNOWN_HOSTS", cfg.KnownHosts},
		{"VAULT_LOCK", cfg.LockPath},
	} {
		if kv[1] == "" {
			missing = append(missing, kv[0])
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("vaultsync: missing required env: %v", missing)
	}
	return cfg, nil
}

// Run executes one pull -> add -> commit-if-changed -> push cycle under an
// exclusive lock on LockPath. The lock serializes this with the host's
// pull-only vault-sync timer so the two never contend on git's index.lock.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	if _, err := os.Stat(filepath.Join(cfg.Dir, ".git")); err != nil {
		return nil, fmt.Errorf("vaultsync: %s is not a git checkout: %w", cfg.Dir, err)
	}

	release, err := cfg.acquireLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	res := &Result{}

	// pull --rebase first so a remote edit reaches the box before we add the
	// new note; a rebase conflict aborts and leaves the working tree clean.
	if out, err := cfg.git(ctx, "pull", "--rebase"); err != nil {
		_, _ = cfg.git(ctx, "rebase", "--abort")
		return nil, fmt.Errorf("vaultsync: git pull --rebase failed: %w\n%s", err, out)
	}
	res.Pulled = true

	// add only the inbox — the sole writable vault surface.
	if out, err := cfg.git(ctx, "add", "inbox/"); err != nil {
		return nil, fmt.Errorf("vaultsync: git add inbox/ failed: %w\n%s", err, out)
	}

	// commit-if-changed: a quiet diff means nothing staged, nothing to push.
	if out, err := cfg.git(ctx, "diff", "--staged", "--quiet"); err == nil {
		return res, nil
	} else if !isGitExitOne(err) {
		return nil, fmt.Errorf("vaultsync: git diff --staged failed: %w\n%s", err, out)
	}

	subject := "inbox: sync agent notes (" + time.Now().UTC().Format("2006-01-02T15:04:05Z") + ")"
	if out, err := cfg.git(ctx, "commit", "-m", subject); err != nil {
		return nil, fmt.Errorf("vaultsync: git commit failed: %w\n%s", err, out)
	}
	res.Committed = true
	res.CommitSubject = subject

	if out, err := cfg.git(ctx, "push"); err != nil {
		return nil, fmt.Errorf("vaultsync: git push failed: %w\n%s", err, out)
	}
	res.Pushed = true
	return res, nil
}

// git runs the git binary in cfg.Dir with a controlled environment: a rebuilt
// GIT_SSH_COMMAND (OpenClaw strips it from configured mcp env), git identity via
// GIT_AUTHOR_*/GIT_COMMITTER_* (so pull --rebase and commit share one identity),
// and HOME set to the bridge's inherited home (the gateway's stateDir, the same
// HOME the vault-sync timer uses) so ssh/git behavior matches the timer's path
// and nothing writes into the vault checkout. The caller's env is not inherited
// wholesale — only PATH (for helper binaries) is passed through.
func (cfg Config) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, cfg.GitBin, args...)
	cmd.Dir = cfg.Dir
	home := os.Getenv("HOME")
	if home == "" {
		home = cfg.Dir
	}
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"GIT_SSH_COMMAND=" + cfg.sshCommand(),
		"GIT_AUTHOR_NAME=" + cfg.AuthorName,
		"GIT_AUTHOR_EMAIL=" + cfg.AuthorEmail,
		"GIT_COMMITTER_NAME=" + cfg.AuthorName,
		"GIT_COMMITTER_EMAIL=" + cfg.AuthorEmail,
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (cfg Config) sshCommand() string {
	return fmt.Sprintf("%s -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=%s",
		cfg.SSHBin, cfg.DeployKey, cfg.KnownHosts)
}

// isGitExitOne reports whether err is an exec.ExitError with code 1, the code
// `git diff --quiet` returns when there ARE staged differences.
func isGitExitOne(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 1
	}
	return false
}

// acquireLock takes an exclusive flock on LockPath, creating it if needed. The
// fd stays open until release. A bounded wait prevents a stuck timer from
// hanging the tool indefinitely.
func (cfg Config) acquireLock(ctx context.Context) (func(), error) {
	timeout := cfg.LockTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}

	f, err := os.OpenFile(cfg.LockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("vaultsync: open lock %s: %w", cfg.LockPath, err)
	}

	var once sync.Once
	release := func() { once.Do(func() { _ = f.Close() }) }

	for {
		// Non-blocking attempt first: the common case is uncontended.
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return release, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			release()
			return nil, fmt.Errorf("vaultsync: flock %s: %w", cfg.LockPath, err)
		}
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			release()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	release()
	return nil, fmt.Errorf("vaultsync: lock %s not acquired within %s", cfg.LockPath, timeout)
}