//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Credentials/recovery in this test are synthetic and private to an expiring,
// loopback-only Synapse. They are never printed or accepted from a real user.
func TestPrivateSearchRealBrowserRustAndNativeSDK(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_ROOMS_BROWSER") != "1" {
		t.Skip("isolated remote Chromium and Synapse required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	target, _ := url.Parse("http://127.0.0.1:18776")
	listener, err := net.Listen("tcp", "127.0.0.1:18775")
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewUnstartedServer(httputil.NewSingleHostReverseProxy(target))
	_ = proxy.Listener.Close()
	proxy.Config.ErrorLog = log.New(io.Discard, "", 0)
	proxy.Listener = listener
	proxy.StartTLS()
	defer proxy.Close()
	actor, channel := uuid.NewString(), uuid.NewString()
	const tenant = "rooms-browser-fixture"
	c, room, recovery, login := searchSDKDevice(ctx, t, proxy.URL, proxy.Client(), actor, tenant, channel)
	defer func() { _ = c.Close() }()
	corpus, wantReply, wantFile := searchCorpus(ctx, t, c, room, actor, tenant, channel)
	if err := c.Backup(ctx); err != nil {
		t.Fatal("native backup before browser recovery", err)
	}
	// Public backup material cannot attest an imported session, even when the
	// REST sender is the expected account and the backup claims verification.
	forgedCipher, _ := anonymousStorageBackup(ctx, t, c.cli, id.RoomID(room), storage.Record{Version: 1, Scope: c.vaultScope(), ObjectID: "forged-browser", RevisionID: "forged-browser", Role: "content", Payload: []byte("{}")})
	forgedSent, err := c.cli.SendMessageEvent(ctx, id.RoomID(room), event.EventEncrypted, forgedCipher)
	if err != nil {
		t.Fatal("artificial public-backup send failed", err)
	}
	forgedReference := EventRef{RoomID: room, EventID: forgedSent.EventID.String()}
	metadata := httptest.NewServer(corpus)
	defer metadata.Close()
	sessions := map[string]any{}
	for _, name := range []string{"desktop", "mobile"} {
		session, err := login("BROWSER_" + name)
		if err != nil {
			t.Fatal("synthetic browser login", err)
		}
		sessions[name] = map[string]any{"user_id": session.UserID, "device_id": session.DeviceID, "access_token": session.AccessToken}
	}
	bootstrap := c.cfg.Bootstrap
	bootstrap.SSOProviderID = "artificial"
	encoded, err := json.Marshal(map[string]any{"artificial": true, "bootstrap": bootstrap, "sessions": sessions, "recovery": recovery, "metadata_url": metadata.URL, "reply_id": wantReply, "file_id": wantFile, "channel": channel, "anonymous_reference": forgedReference})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fixturePath, resultPath := filepath.Join(dir, "fixture.json"), filepath.Join(dir, "result.json")
	if err := os.WriteFile(fixturePath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	clear(encoded)
	cfg := c.cfg
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	runner := filepath.Join("..", "..", "..", "test", "e2e", "rooms-search-browser.mjs")
	command := exec.CommandContext(ctx, "bun", runner, fixturePath, resultPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("real Rust browser search probe failed: %v\n%s", err, output)
	}
	resultBytes, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipts map[string]EventRef
	if json.Unmarshal(resultBytes, &receipts) != nil || len(receipts) != 2 {
		t.Fatal("invalid browser receipts")
	}
	c, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	for name, reference := range receipts {
		if reference.RoomID != room || reference.EventID == "" {
			t.Fatal("browser receipt changed room")
		}
		decrypted, err := c.Read(ctx, reference)
		if err != nil {
			t.Fatal("native could not decrypt browser message", name, err)
		}
		content, err := DecodeMessage(decrypted)
		expected := "browser-" + name + " encrypted reply"
		if err != nil || content.Body != expected {
			t.Fatal("browser/native content changed", name, err)
		}
		wire, err := c.cli.GetEvent(ctx, id.RoomID(room), id.EventID(reference.EventID))
		if err != nil || wire.Type != event.EventEncrypted {
			t.Fatal("browser message was not an encrypted Matrix event", name, err)
		}
		raw, err := json.Marshal(wire)
		if err != nil || bytes.Contains(raw, []byte(expected)) {
			t.Fatal("browser content reached Synapse in plaintext", name, err)
		}
	}
	t.Log("real desktop/mobile Chromium + Rust SDK: recover native backup, search 207 encrypted events with deep reply and file/link/saved/pinned operators, send to native; all content encrypted on Synapse")
}
