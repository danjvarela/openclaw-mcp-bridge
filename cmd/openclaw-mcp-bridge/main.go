// openclaw-mcp-bridge is a stdio MCP server exposing the tracer tool `ping`.
//
// It speaks JSON-RPC 2.0 over stdin/stdout (one JSON object per line) using the
// official Go MCP SDK. All logging goes to stderr; stdout is reserved for
// JSON-RPC responses.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danjvarela/openclaw-mcp-bridge/internal/vaultsync"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// version is the server version reported in `initialize`. Override at build
// time with -ldflags "-X main.version=...".
var version = "0.1.0"

func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "openclaw-mcp-bridge",
		Version: version,
	}, &mcp.ServerOptions{
		// Tools only: no resources, no prompts, no logging. Advertise the
		// tools capability as an empty object (no listChanged).
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})

	server.AddTool(&mcp.Tool{
		Name:        "ping",
		Description: "Tracer no-op. Returns pong.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, ping)

	server.AddTool(&mcp.Tool{
		Name:        "sync_notes",
		Description: "Push newly captured vault inbox notes to GitHub immediately. Runs pull --rebase, commits any new inbox/ files, and pushes. Call this right after writing a note so the user's other devices see it without waiting for the host pull timer.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, syncNotes)

	if err := server.Run(context.Background(), newStdioTransport()); err != nil {
		// A clean shutdown after the client closes stdin surfaces as one of these.
		if errors.Is(err, os.ErrClosed) || errors.Is(err, io.EOF) {
			return
		}
		log.Fatalf("server: %v", err)
	}
}

func ping(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "pong"},
		},
	}, nil
}

// syncNotes runs the vault git write cycle on the host (pull -> add inbox/ ->
// commit-if-changed -> push) under a shared lock so a note the agent just wrote
// reaches GitHub immediately instead of waiting for the host pull timer. The
// cycle config comes from VAULT_* env (see vaultsync.FromEnv).
func syncNotes(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := vaultsync.FromEnv(os.Getenv)
	if err != nil {
		return toolError("sync_notes not configured: %v", err), nil
	}
	res, err := vaultsync.Run(ctx, cfg)
	if err != nil {
		return toolError("sync_notes failed: %v", err), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: res.String()}},
	}, nil
}

// toolError returns a CallToolResult carrying an error as text. Tool errors are
// surfaced to the model as content, not as JSON-RPC errors, so the agent can
// read the failure reason and report it to the user.
func toolError(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
		IsError: true,
	}
}

// writeQuiescence is how long the reader waits, after stdin reaches EOF, for
// the most recent response to be written to stdout before propagating EOF to
// the SDK.
//
// The SDK rejects outgoing writes once it has observed EOF on the read side.
// A real MCP client keeps stdin open until it has read every response, so EOF
// arrives only after all writes are done and this gate is a no-op. A bare-pipe
// test that closes stdin right after sending its requests would otherwise drop
// responses: the read side EOFs before the SDK's asynchronous handlers flush.
// This gate holds EOF open until writes have quiesced, so responses survive.
//
// The wait is measured from the last write, so it adapts to slow tools rather
// than imposing a fixed cap.
const writeQuiescence = 20 * time.Millisecond

// stdioTransport is a stdin/stdout [mcp.IOTransport] pair that gates read-side
// EOF on write quiescence, preventing responses from being dropped when stdin
// closes before the handlers have flushed (see writeQuiescence).
type stdioTransport struct {
	lastWriteNs atomic.Int64
	closeOnce   sync.Once
}

func newStdioTransport() *mcp.IOTransport {
	t := &stdioTransport{}
	return &mcp.IOTransport{Reader: t, Writer: t}
}

func (t *stdioTransport) Read(p []byte) (int, error) {
	n, err := os.Stdin.Read(p)
	if err != nil {
		t.awaitQuiescence()
	}
	return n, err
}

func (t *stdioTransport) Write(p []byte) (int, error) {
	n, err := os.Stdout.Write(p)
	if n > 0 {
		t.lastWriteNs.Store(time.Now().UnixNano())
	}
	return n, err
}

func (t *stdioTransport) Close() error {
	// IOTransport wires the same value as both Reader and Writer, so the SDK
	// closes it twice; guard against the double close.
	t.closeOnce.Do(func() {
		_ = os.Stdin.Close()
		_ = os.Stdout.Close()
	})
	return nil
}

func (t *stdioTransport) awaitQuiescence() {
	eofNs := time.Now().UnixNano()
	for {
		ref := t.lastWriteNs.Load()
		if ref == 0 {
			ref = eofNs
		}
		if time.Now().UnixNano()-ref >= int64(writeQuiescence) {
			return
		}
		time.Sleep(writeQuiescence / 4)
	}
}
