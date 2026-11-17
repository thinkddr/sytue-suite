// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type calendarRemoteFixtureSession string

func (s calendarRemoteFixtureSession) AccessToken(context.Context) (string, error) {
	return string(s), nil
}
func (calendarRemoteFixtureSession) Invalidate(string) {}

func calendarSourceFixtureEndpoint(raw string) (*url.URL, error) {
	base, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid disposable source endpoint")
	}
	ip := net.ParseIP(base.Hostname())
	port, err := strconv.Atoi(base.Port())
	if err != nil || ip == nil || !ip.IsLoopback() || port < 1024 || port > 65535 || base.Scheme != "http" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return nil, errors.New("source fixture endpoint must be an ephemeral loopback HTTP server")
	}
	base.Path = ""
	return base, nil
}

func TestCalendarSourceFixtureEndpointGuards(t *testing.T) {
	if _, err := calendarSourceFixtureEndpoint("http://127.0.0.1:42345"); err != nil {
		t.Fatal("ephemeral loopback source was rejected")
	}
	for _, raw := range []string{"http://192.0.2.1:42345", "http://localhost:42345", "https://127.0.0.1:42345", "http://127.0.0.1:80", "http://user:synthetic@127.0.0.1:42345", "http://127.0.0.1:42345/?token=synthetic"} {
		if _, err := calendarSourceFixtureEndpoint(raw); err == nil {
			t.Fatal("non-fixture endpoint passed the guard")
		}
	}
}

// This fixture is intentionally selected by an explicit flag, not ordinary test
// discovery. Its caller owns a disposable loopback server and synthetic rows.
// No stored session, keyring, token environment or production URL is accepted.
func TestCalendarSourceRemoteFixture(t *testing.T) {
	if os.Getenv("SYTUE_CALENDAR_SOURCE_FIXTURE") != "1" {
		t.Skip("requires an explicit disposable source fixture")
	}
	base, err := calendarSourceFixtureEndpoint(os.Getenv("SYTUE_CALENDAR_SOURCE_FIXTURE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	epoch := os.Getenv("SYTUE_CALENDAR_SOURCE_FIXTURE_EPOCH")
	if !storage.MigrationEpoch(epoch) {
		t.Fatal("invalid disposable source epoch")
	}
	var row storage.StreamSourceRow
	if json.Unmarshal([]byte(os.Getenv("SYTUE_CALENDAR_SOURCE_FIXTURE_ROW")), &row) != nil || row.Validate() != nil || row.Kind != "object" || row.Size < 40<<10 || row.Size > 2<<20 {
		t.Fatal("invalid synthetic source descriptor")
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	transport := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	client := NewCalendars(base.String(), calendarRemoteFixtureSession("verified-test-account"), transport)
	state, err := client.PrivateStorageState(ctx, scope)
	if err != nil || state.Validate(scope) != nil || state.Mode != "frozen" || state.Epoch != epoch || state.Rows != 3 || state.PrivateIngressReady {
		t.Fatal("disposable historical source changed state or granted continuing ingress")
	}
	page, err := client.PrivateStorageSourcePage(ctx, scope, epoch, storage.LegacySourceCursor{})
	if err != nil || len(page.Rows) != 3 {
		t.Fatal("disposable source page changed")
	}
	found := false
	for _, candidate := range page.Rows {
		if candidate == row {
			found = true
		}
	}
	if !found {
		t.Fatal("disposable SQL source descriptor changed")
	}
	body, err := client.OpenPrivateStorageSource(ctx, scope, epoch, row)
	if err != nil {
		t.Fatal("disposable original could not be opened")
	}
	raw, err := io.ReadAll(io.LimitReader(body, row.Size+1))
	_ = body.Close()
	sum := sha256.Sum256(raw)
	var original struct {
		UID string `json:"uid"`
	}
	if err != nil || int64(len(raw)) != row.Size || hex.EncodeToString(sum[:]) != row.SHA256 || json.Unmarshal(raw, &original) != nil || original.UID != "http-source" {
		t.Fatal("disposable original bytes or synthetic identity changed")
	}
	_, err = client.BeginPrivateStorageMigration(ctx, scope, epoch)
	var failure *Error
	if !errors.As(err, &failure) || failure.Status != http.StatusServiceUnavailable || failure.Code() != "private_calendar_ingress_required" {
		t.Fatal("frozen history enabled public continuing ingress")
	}
	other := NewCalendars(base.String(), calendarRemoteFixtureSession("other-verified-account"), transport)
	_, err = other.PrivateStorageState(ctx, scope)
	if !errors.As(err, &failure) || failure.Status != http.StatusConflict || failure.Code() != "storage_identity_changed" {
		t.Fatal("another authenticated account reused the captured source scope")
	}
	t.Log("CALENDAR_NATIVE_REMOTE_FIXTURE_PASS")
}
