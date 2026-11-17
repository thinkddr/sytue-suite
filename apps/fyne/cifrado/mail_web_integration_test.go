// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package cifrado

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

// This browser phase reuses actual SQL capture, Matrix identity cutover and
// independently recovered native SSSS. NativeFull keeps the separate ingress
// ACK/SMTP/UI cases. No isolated or empty migration state is fabricated.
func verifyMailBrowserSession(ctx context.Context, t *testing.T, vector matrixprooftest.IngressDeviceVector, configuration mx.Config, recovery, apiOrigin, blobOrigin, knownID string, facade *api.Mail, rooms *api.Rooms, native *MailApplicationRepository) {
	t.Helper()
	endpoint, err := matrixprooftest.DisposableHTTPOrigin(vector.SynapseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = matrixprooftest.DisposableHTTPOrigin(apiOrigin); err != nil {
		t.Fatal(err)
	}
	if _, err = matrixprooftest.DisposableHTTPOrigin(blobOrigin); err != nil {
		t.Fatal(err)
	}
	cli, err := mautrix.NewClient(endpoint.String(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	login, err := cli.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: vector.UserID}, Password: "artificial-disposable-ingress-password", DeviceID: "ARTIFICIAL_MAIL_BROWSER"})
	if err != nil {
		t.Fatal("browser SDK login failed", err)
	}
	directory := t.TempDir()
	fixturePath := filepath.Join(directory, "fixture.json")
	resultPath := filepath.Join(directory, "result.json")
	readyPath := filepath.Join(directory, "runner-ready.json")
	matrixProxy := httputil.NewSingleHostReverseProxy(endpoint)
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/_matrix/") || strings.HasPrefix(r.URL.Path, "/_synapse/") {
			matrixProxy.ServeHTTP(w, r)
			return
		}
		raw, err := os.ReadFile(readyPath)
		if err != nil {
			http.Error(w, "browser runner unavailable", http.StatusServiceUnavailable)
			return
		}
		var ready struct {
			Origin string `json:"origin"`
		}
		err = json.Unmarshal(raw, &ready)
		clear(raw)
		origin, parseErr := matrixprooftest.DisposableHTTPOrigin(ready.Origin)
		if err != nil || parseErr != nil {
			http.Error(w, "invalid browser runner", http.StatusServiceUnavailable)
			return
		}
		httputil.NewSingleHostReverseProxy(origin).ServeHTTP(w, r)
	}))
	defer proxy.Close()
	_, serverName, ok := strings.Cut(vector.UserID, ":")
	if !ok || serverName != "storage.test:18746" {
		t.Fatal("Mail browser proof requires its dedicated pre-provisioned Matrix server name")
	}
	if err = proxy.Listener.Close(); err != nil {
		t.Fatal(err)
	}
	proxy.Listener, err = net.Listen("tcp", "127.0.0.1:18746")
	if err != nil {
		t.Fatal(err)
	}
	proxy.StartTLS()
	if rooms == nil {
		t.Fatal("browser requires the captured native Rooms client")
	}
	bootstrap, err := rooms.MatrixStorageBootstrapForIdentity(ctx, "mail", storage.IsolatedIdentity)
	if err != nil {
		t.Fatal("browser metadata must come from the real Rooms bootstrap", err)
	}
	nativeBootstrap := configuration.Bootstrap
	if bootstrap.Actor != vector.Scope.Subject || bootstrap.Tenant != vector.Scope.Tenant ||
		bootstrap.StorageProduct != "mail" || bootstrap.StorageIdentityVersion != storage.IsolatedIdentity ||
		bootstrap.MatrixUserID != nativeBootstrap.MatrixUserID || bootstrap.ControllerUserID != nativeBootstrap.ControllerUserID ||
		bootstrap.StorageRoomID != nativeBootstrap.StorageRoomID || !slices.Equal(bootstrap.StorageRoomIDs, nativeBootstrap.StorageRoomIDs) ||
		bootstrap.HomeserverURL != "https://"+serverName || bootstrap.SSOProviderID == "" || bootstrap.Channels == nil || len(bootstrap.Channels) != 0 {
		t.Fatal("real browser bootstrap differs from the recovered native identity")
	}
	expectedBody, err := facade.Raw(ctx, knownID)
	if err != nil {
		t.Fatal(err)
	}
	expected, readErr := io.ReadAll(expectedBody)
	closeErr := expectedBody.Close()
	defer clear(expected)
	if readErr != nil || closeErr != nil {
		t.Fatal(readErr, closeErr)
	}
	fixture := map[string]any{
		"artificial": true, "bootstrap": bootstrap, "scope": vector.Scope, "recovery": recovery,
		"session":    map[string]any{"user_id": login.UserID, "device_id": login.DeviceID, "access_token": login.AccessToken, "storage_scope": vector.Scope},
		"api_origin": apiOrigin, "blobs_origin": blobOrigin, "known_id": knownID, "known_mime": expected,
		"base_path": "/mail", "native_migration_completed": true,
		"browser_origin": bootstrap.HomeserverURL, "ready_path": readyPath,
		"sytue_access_token": "synthetic-live-device-session", "sytue_expires_at": time.Now().Add(15 * time.Minute).Unix(), "bff_client_id": "sytue-mail",
		"user": map[string]any{"id": vector.Scope.Subject, "tenant": vector.Scope.Tenant, "email": "synthetic@example.invalid", "name": "Artificial Mail owner", "roles": []string{"member"}},
	}
	if os.Getenv("SYTUE_MAIL_WEB_DEBUG") == "1" {
		deadline, bounded := ctx.Deadline()
		if !bounded {
			t.Fatal("debug hold requires the composite deadline")
		}
		fixture["debug_deadline_ms"] = deadline.UnixMilli()
	}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(fixturePath, raw, 0o600)
	clear(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = native.Drain(ctx); err != nil {
		t.Fatal("native producers did not drain before strict browser phase", err)
	}
	if pending, pendingErr := native.Pending(ctx); pendingErr != nil || pending.Count != 0 {
		t.Fatal("native pending work cannot be skipped for browser proof", pendingErr)
	}
	if os.Getenv("SYTUE_MAIL_NATIVE_STRICT_PROOF") == "1" {
		t.Log("MAIL_NATIVE_STRICT_ORIGIN_REAL_DRAINED_PENDING_ZERO")
	} else {
		t.Log("MAIL_NATIVE_COMPAT_ORIGIN_REAL_DRAINED_PENDING_ZERO")
	}
	phase, err := http.NewRequestWithContext(ctx, http.MethodPost, apiOrigin+"/fixture/browser-phase", nil)
	if err != nil {
		t.Fatal(err)
	}
	phase.Header.Set("Authorization", "Bearer synthetic-live-device-session")
	phaseReply, err := http.DefaultClient.Do(phase)
	if err != nil {
		t.Fatal(err)
	}
	_ = phaseReply.Body.Close()
	if phaseReply.StatusCode != http.StatusNoContent {
		t.Fatal("strict browser origin did not activate", phaseReply.StatusCode)
	}
	command := exec.CommandContext(ctx, "bun", filepath.Join("..", "..", "..", "test", "e2e", "mail-private-session.mjs"), fixturePath, resultPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("actual Mail browser session failed: %v\n%s", err, output)
	}
	resultBytes, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(resultBytes)
	var result struct {
		MessageID         string `json:"message_id"`
		MIME              []byte `json:"mime"`
		Paused            bool   `json:"paused"`
		PendingVerified   bool   `json:"pending_verified"`
		ClosedAndRetained bool   `json:"closed_and_retained"`
	}
	if json.Unmarshal(resultBytes, &result) != nil || result.MessageID == "" || len(result.MIME) == 0 || !result.Paused || !result.PendingVerified || !result.ClosedAndRetained {
		t.Fatal("browser did not prove an admitted Mail engine and retention barrier")
	}
	defer clear(result.MIME)
	if err = native.Resume(); err != nil {
		t.Fatal("native readback scope could not resume", err)
	}
	// Read the browser's exact original through the existing fresh native SDK,
	// independently from its browser repository and persistent encrypted journal.
	deadline := time.Now().Add(20 * time.Second)
	for {
		reader, readErr := facade.Raw(ctx, result.MessageID)
		if readErr == nil {
			actual, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			valid := readErr == nil && closeErr == nil && bytes.Equal(actual, result.MIME)
			clear(actual)
			if !valid {
				t.Fatal("browser Mail original changed on fresh native readback", readErr, closeErr)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fresh native did not recover the browser Mail revision", readErr)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Log("MAIL_BROWSER_MANAGER_ACTUAL_PG_SSSS_NATIVE_MIME_RETAINED_DRAIN_GREEN")
}
