package vaultsync_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/danjvarela/openclaw-mcp-bridge/internal/vaultsync"
)

// skipIfNoGit skips when git is unavailable in the test PATH.
func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
}

// Test git identity for the local clones.
const (
	testAuthorName  = "test-bot"
	testAuthorEmail = "test@example.com"
)

// makeRemoteRepo creates a bare repo and returns its path.
func makeRemoteRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustRun(t, "", "git", "init", "--bare", dir)
	return dir
}

// makeClone clones the bare remote into a checkout with an initial inbox/
// commit, returning the checkout path. The clone shares the local git identity.
func makeClone(t *testing.T, remote, name, email string) string {
	t.Helper()
	clone := t.TempDir()
	mustRun(t, "", "git", "clone", remote, clone)
	mustRun(t, clone, "git", "config", "user.name", name)
	mustRun(t, clone, "git", "config", "user.email", email)
	mustRun(t, clone, "git", "config", "commit.gpgsign", "false")
	_ = os.Mkdir(filepath.Join(clone, "inbox"), 0o755)
	mustWriteFile(t, filepath.Join(clone, "inbox", ".gitkeep"), "")
	mustRun(t, clone, "git", "add", "inbox/.gitkeep")
	mustRun(t, clone, "git", "-c", "commit.gpgsign=false", "commit", "-m", "seed")
	mustRun(t, clone, "git", "push", "origin", "HEAD")
	return clone
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

// cfgFor builds a vaultsync.Config for a local-file-origin clone so the cycle
// runs without ssh or network. GIT_SSH_COMMAND is unused for file remotes.
func cfgFor(t *testing.T, clone, lock string) vaultsync.Config {
	t.Helper()
	return vaultsync.Config{
		Dir:         clone,
		DeployKey:   "/nonexistent-key", // unused: file:// remote, not ssh
		KnownHosts:  "/nonexistent-hosts",
		AuthorName:  "bot",
		AuthorEmail: "bot@example.com",
		GitBin:      "git",
		SSHBin:      "ssh",
		LockPath:    lock,
		LockTimeout: 5 * time.Second,
	}
}

// TestRunCommitsAndPushesNewNote verifies the happy path: an inbox file the
// agent wrote is committed and pushed, and a second sync with nothing new is a
// no-op commit.
func TestRunCommitsAndPushesNewNote(t *testing.T) {
	skipIfNoGit(t)
	name, email := testAuthorName, testAuthorEmail
	remote := makeRemoteRepo(t)
	clone := makeClone(t, remote, name, email)
	lock := filepath.Join(t.TempDir(), "vault.lock")

	cfg := cfgFor(t, clone, lock)

	// Agent writes a new note into inbox/.
	mustWriteFile(t, filepath.Join(clone, "inbox", "2026-09-19-0900-hello.md"), "# hello\n")

	res, err := vaultsync.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Committed || !res.Pushed {
		t.Fatalf("expected committed+pushed, got %+v", res)
	}

	// A fresh clone from the remote should contain the pushed note.
	verify := t.TempDir()
	mustRun(t, verify, "git", "clone", remote, filepath.Join(verify, "v"))
	if _, err := os.Stat(filepath.Join(verify, "v", "inbox", "2026-09-19-0900-hello.md")); err != nil {
		t.Fatalf("note not pushed to remote: %v", err)
	}

	// Second sync with no new notes: pulled, not committed, not pushed.
	res2, err := vaultsync.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if res2.Committed {
		t.Fatalf("unexpected commit on empty sync: %+v", res2)
	}
}

// TestRunPullsRemoteEditBeforePush verifies pull --rebase lands a remote edit
// before the local note is committed, so the push includes both.
func TestRunPullsRemoteEditBeforePush(t *testing.T) {
	skipIfNoGit(t)
	name, email := testAuthorName, testAuthorEmail
	remote := makeRemoteRepo(t)
	clone := makeClone(t, remote, name, email)
	lock := filepath.Join(t.TempDir(), "vault.lock")

	// A second device commits a remote note and pushes.
	other := t.TempDir()
	mustRun(t, other, "git", "clone", remote, filepath.Join(other, "o"))
	mustRun(t, other, "git", "-C", filepath.Join(other, "o"), "config", "user.name", name)
	mustRun(t, other, "git", "-C", filepath.Join(other, "o"), "config", "user.email", email)
	mustWriteFile(t, filepath.Join(other, "o", "inbox", "remote-note.md"), "# from another device\n")
	mustRun(t, other, "git", "-C", filepath.Join(other, "o"), "add", "inbox/remote-note.md")
	mustRun(t, other, "git", "-C", filepath.Join(other, "o"), "-c", "commit.gpgsign=false", "commit", "-m", "remote note")
	mustRun(t, other, "git", "-C", filepath.Join(other, "o"), "push", "origin", "HEAD")

	// Agent writes a local note and syncs; pull --rebase should fetch the remote note first.
	mustWriteFile(t, filepath.Join(clone, "inbox", "local-note.md"), "# local\n")
	cfg := cfgFor(t, clone, lock)
	res, err := vaultsync.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Committed || !res.Pushed {
		t.Fatalf("expected committed+pushed, got %+v", res)
	}
	if _, err := os.Stat(filepath.Join(clone, "inbox", "remote-note.md")); err != nil {
		t.Fatalf("remote note not pulled into checkout: %v", err)
	}
	// Both notes should now be on the remote.
	verify := t.TempDir()
	mustRun(t, verify, "git", "clone", remote, filepath.Join(verify, "v"))
	for _, f := range []string{"remote-note.md", "local-note.md"} {
		if _, err := os.Stat(filepath.Join(verify, "v", "inbox", f)); err != nil {
			t.Fatalf("%s not on remote after push: %v", f, err)
		}
	}
}

// TestRunMissingEnv verifies FromEnv rejects an incomplete config.
func TestRunMissingEnv(t *testing.T) {
	_, err := vaultsync.FromEnv(func(k string) string { return "" })
	if err == nil {
		t.Fatal("expected error for empty env")
	}
	if !strings.Contains(err.Error(), "VAULT_DIR") {
		t.Fatalf("expected VAULT_DIR in error, got: %v", err)
	}
}

// TestRunNotAGitCheckout verifies Run rejects a dir without .git.
func TestRunNotAGitCheckout(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	lock := filepath.Join(t.TempDir(), "vault.lock")
	cfg := vaultsync.Config{
		Dir: dir, DeployKey: "x", KnownHosts: "y", LockPath: lock,
		GitBin: "git", SSHBin: "ssh",
		AuthorName: "bot", AuthorEmail: "bot@example.com",
		LockTimeout: 2 * time.Second,
	}
	if _, err := vaultsync.Run(context.Background(), cfg); err == nil {
		t.Fatal("expected error for non-git dir")
	}
}

// TestLockSerializes verifies two concurrent Runs against the same lock file
// never overlap (the second blocks until the first releases), using a slow
// remote hook is impractical here; instead we confirm the lock is exclusive by
// running a Run while holding the lock from outside.
func TestLockSerializes(t *testing.T) {
	skipIfNoGit(t)
	name, email := testAuthorName, testAuthorEmail
	remote := makeRemoteRepo(t)
	clone := makeClone(t, remote, name, email)
	lock := filepath.Join(t.TempDir(), "vault.lock")

	// Hold the lock externally with an advisory flock; Run should time out
	// rather than proceed, since flock contention is against other flock holders.
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open lock: %v", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("flock: %v", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	cfg := cfgFor(t, clone, lock)
	cfg.LockTimeout = 200 * time.Millisecond
	start := time.Now()
	_, err = vaultsync.Run(context.Background(), cfg)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected lock-acquire timeout, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Run did not respect LockTimeout: took %v", elapsed)
	}
}