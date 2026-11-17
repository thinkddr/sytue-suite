// SPDX-License-Identifier: Apache-2.0
//go:build integration

package casthttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/casttest"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

func TestPrivateCastSourcePGHTTPNativeClientExactBodyAndCorruption(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant})
	video := uuid.New()
	key := video.String() + "/original"
	original := bytes.Repeat([]byte{0, 1, 2, 3, 0xfe, 0xff}, 128<<10)
	if err := h.PoolSinRLS(t).InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO videos(id,tenant,owner_sub,kind,status,visibility,title,description,original_key,bytes_original,bytes_total)VALUES($1,'acme','alice','upload','ready','private','Original / 漢字 é','retained metadata',$2,$3,$3)`, video, key, len(original))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	objects := &casttest.Objects{Bodies: map[string][]byte{key: original}}
	source, err := store.NewPrivateSource(st, objects)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := authn.Claims{Tenant: scope.Tenant, Subject: scope.Subject}
			switch r.Header.Get("Authorization") {
			case "Bearer source-alice":
			case "Bearer source-bob":
				claims.Subject = "bob"
			default:
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(authn.Into(tenancy.Into(r.Context(), tenancy.Tenant{Slug: scope.Tenant}), claims)))
		})
	})
	router.Route("/api/v1", func(r chi.Router) { mountCastPrivateSource(r, source) })
	router.Post("/fixture/corrupt", func(w http.ResponseWriter, _ *http.Request) {
		objects.Set(key, bytes.Repeat([]byte{0x5a}, len(original)))
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(router)
	defer server.Close()
	pgtest.RunNativeFixture(ctx, t, pgtest.NativeFixture{Module: "../../../../apps/nucleo", Package: "./api", Test: "TestCastSourceRemoteFixture", Marker: "CAST_NATIVE_REMOTE_FIXTURE_PASS", Timeout: 2 * time.Minute, Env: map[string]string{"SYTUE_CAST_SOURCE_URL": server.URL}})
	t.Log("CAST_REAL_PG_HTTP_NATIVE_SOURCE_PASS")
}
