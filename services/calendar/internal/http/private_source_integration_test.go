// SPDX-License-Identifier: Apache-2.0
//go:build integration

package calendarhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

func TestCalendarPrivateSourceRealPGHTTPAndNativeClient(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: scope.Tenant}), authn.Claims{Tenant: scope.Tenant, Subject: scope.Subject, Email: "alice@example.invalid"})
	c, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: scope.Subject, Name: "original"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2027, 3, 29, 7, 0, 0, 0, time.UTC)
	description := strings.Repeat("Original / 漢字 é ", 3000)
	data, err := calendarical.EncodeEvent(domain.Event{UID: "http-source", Summary: "Original", Description: description, DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, object, err := st.CreateEvent(ctx, domain.EventInput{CalendarID: c.ID, UID: "http-source", ResourceName: "http-source.ics", ICalData: data, Summary: "Original", Description: description, DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject := "alice"
			if r.Header.Get("Authorization") == "Bearer other-verified-account" {
				subject = "bob"
			}
			claims := authn.Claims{Tenant: "acme", Subject: subject, Email: subject + "@example.invalid"}
			next.ServeHTTP(w, r.WithContext(authn.Into(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}), claims)))
		})
	})
	router.Route("/api/v1", func(api chi.Router) { mountCalendarPrivateSource(api, st) })
	server := httptest.NewServer(router)
	defer server.Close()
	request := func(method, path, body string) (int, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, method, server.URL+"/api/v1/private-storage/migration"+path, strings.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		value, err := storage.EncodeScopeHeader(scope)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set(storage.ScopeHeader, value)
		req.Header.Set("Authorization", "Bearer verified-test-account")
		response, err := server.Client().Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer func() { _ = response.Body.Close() }()
		raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		return response.StatusCode, raw, err
	}
	status, raw, err := request(http.MethodGet, "", "")
	var state storage.AppMigrationState
	if err != nil || status != http.StatusOK || json.Unmarshal(raw, &state) != nil || state.Mode != "legacy" || state.Rows != 0 || state.PrivateIngressReady {
		t.Fatal("legacy state changed before device ingress", status, err)
	}
	epoch := uuid.New()
	status, raw, err = request(http.MethodPost, "", `{"epoch":"`+epoch.String()+`"}`)
	if err != nil || status != http.StatusServiceUnavailable || !bytes.Contains(raw, []byte("private_calendar_ingress_required")) {
		t.Fatal("public HTTP began a partial migration", status, err)
	}
	status, _, err = request(http.MethodGet, "/"+epoch.String()+"/rows", "")
	if err != nil || status != http.StatusConflict {
		t.Fatal("legacy source was published without a frozen epoch", status, err)
	}
	var original []byte
	// Fixture-only staging bypasses the deliberately closed public Begin. The
	// production Calendar capture/preflight is covered in store's real PG tests.
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT calendar_private_lock_identities('alice',ARRAY['alice@example.invalid']),set_config('app.owner_sub','alice',true)`); err != nil {
			return err
		}
		captured, err := pg.BeginPrivateStorageSnapshot(ctx, tx, pg.PrivateSnapshotConfig{Scope: scope, Epoch: epoch, Capture: func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
			snapshot, err := store.ReadPrivateStorageInventory(ctx, tx, scope)
			if err != nil {
				return pg.PrivateSnapshotCounts{}, err
			}
			rows := snapshot.Rows()
			var kinds, ids []string
			var payloads [][]byte
			for _, row := range rows {
				kinds = append(kinds, row.Kind)
				ids = append(ids, row.ID)
				payloads = append(payloads, row.JSON)
				if row.Kind == "object" && row.ID == object.ID.String() {
					original = bytes.Clone(row.JSON)
				}
			}
			inserted, err := tx.Exec(ctx, `INSERT INTO calendar_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload)SELECT $1,$2,$3,kind,id,payload FROM unnest($4::text[],$5::text[],$6::bytea[])source(kind,id,payload)`, scope.Tenant, scope.Subject, epoch, kinds, ids, payloads)
			if err != nil {
				return pg.PrivateSnapshotCounts{}, err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO calendar_private_storage_aliases(tenant,owner_sub,epoch,address)VALUES('acme','alice',$1,'alice@example.invalid')`, epoch); err != nil {
				return pg.PrivateSnapshotCounts{}, err
			}
			return pg.PrivateSnapshotCounts{Rows: inserted.RowsAffected()}, nil
		}})
		if err != nil {
			return err
		}
		if captured.Rows != 3 {
			return errors.New("fixture inventory was incomplete")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	status, raw, err = request(http.MethodGet, "", "")
	if err != nil || status != http.StatusOK || json.Unmarshal(raw, &state) != nil || state.Mode != "frozen" || state.Rows != 3 || state.PrivateIngressReady {
		t.Fatal("frozen fixture invented continuing ingress", status, err)
	}
	status, raw, err = request(http.MethodGet, "/"+epoch.String()+"/rows", "")
	var page storage.StreamSourcePage
	if err != nil || status != http.StatusOK || json.Unmarshal(raw, &page) != nil || len(page.Rows) != 3 {
		t.Fatal("source descriptors changed over HTTP", status, err)
	}
	var descriptor storage.StreamSourceRow
	for _, row := range page.Rows {
		if row.Kind == "object" && row.ID == object.ID.String() {
			descriptor = row
		}
	}
	sum := sha256.Sum256(original)
	if descriptor.Size != int64(len(original)) || descriptor.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("SQL original commitment changed before native source")
	}
	runCalendarNativeFixture(ctx, t, server.URL, epoch, descriptor)
}

// Keep native keyring/DBus dependencies in apps/nucleo's module. The subprocess
// inherits CI toolchain/GOFLAGS, receives synthetic routing only, and cannot
// receive the PostgreSQL credential or a production endpoint from this fixture.
func runCalendarNativeFixture(ctx context.Context, t *testing.T, base string, epoch uuid.UUID, row storage.StreamSourceRow) {
	t.Helper()
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	pgtest.RunNativeFixture(ctx, t, pgtest.NativeFixture{
		Module: "../../../../apps/nucleo", Package: "./api", Test: "TestCalendarSourceRemoteFixture", Marker: "CALENDAR_NATIVE_REMOTE_FIXTURE_PASS", Timeout: time.Minute,
		Env: map[string]string{"SYTUE_CALENDAR_SOURCE_FIXTURE": "1", "SYTUE_CALENDAR_SOURCE_FIXTURE_URL": base, "SYTUE_CALENDAR_SOURCE_FIXTURE_EPOCH": epoch.String(), "SYTUE_CALENDAR_SOURCE_FIXTURE_ROW": string(encoded)},
	})
	t.Log("CALENDAR_PG_HTTP_NATIVE_FIXTURE_PASS rows=3 ingress_ready=false")
}
