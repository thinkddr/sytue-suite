// SPDX-License-Identifier: Apache-2.0
//go:build integration

package http

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

func TestPrivatePhotosPostgresHTTPToSDKAndFreshDevice(t *testing.T) {
	privatePhotosPostgresHTTPToSDKAndFreshDevice(t, false)
}

func TestPrivatePhotosLargePostgresHTTPToSDKAndFreshDevice(t *testing.T) {
	privatePhotosPostgresHTTPToSDKAndFreshDevice(t, true)
}

func privatePhotosPostgresHTTPToSDKAndFreshDevice(t *testing.T, large bool) {
	t.Helper()
	upstream := os.Getenv("SYTUE_PHOTOS_SOURCE_FIXTURE_SYNAPSE_URL")
	if upstream == "" {
		t.Skip("requires isolated disposable remote Synapse")
	}
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "photos"}
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant})
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	driveID := uuid.NewString()
	asset, err := st.Ingerir(ctx, domain.Activo{Duenyo: scope.Subject, DriveID: driveID, Nombre: "retained.png", Tipo: "image/png", Bytes: int64(len("private baseline content version two")), Ancho: 16, Alto: 12, Orientacion: 1})
	if err != nil {
		t.Fatal(err)
	}
	wantRows := int64(5)
	if large {
		if _, err := st.GuardarReceta(ctx, asset.ID, scope.Subject, domain.RecetaVacia()); err != nil {
			t.Fatal(err)
		}
		// Two individually valid entities exceed32MiB together. Unknown recipe
		// JSON is preserved as bytes rather than normalized through Go floats.
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE assets SET camera=repeat('retained EXIF / ',1400000) WHERE tenant=$1 AND id=$2`, scope.Tenant, asset.ID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE edits SET recipe=recipe||jsonb_build_object('future_private',repeat('retained unknown recipe / ',900000),'precise_integer',900719925474099312345::numeric) WHERE tenant=$1 AND asset_id=$2 AND owner_sub=$3`, scope.Tenant, asset.ID, scope.Subject)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		wantRows++
	}
	bodies := map[string][]byte{}
	for _, recipe := range []string{"old-recipe", "new-recipe"} {
		key, err := (model.Derivada{ActivoID: asset.ID, Huella: recipe, Variante: model.VarianteMiniatura, Formato: "png"}).Clave()
		if err != nil {
			t.Fatal(err)
		}
		bodies[key] = []byte("exact historical thumbnail " + recipe + "\x00\xff")
	}
	objects := photosRetainedS3Server(t, bodies)
	client, err := objectstore.New(objectstore.Config{Endpoint: strings.TrimPrefix(objects.URL, "http://"), Region: "eu-test-1", AccessKey: "synthetic", SecretKey: "synthetic", Bucket: "retained-photos"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.NewPrivateSource(st, client)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer synthetic-notes-source" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			requestCtx := tenancy.Into(r.Context(), tenancy.Tenant{Slug: scope.Tenant})
			requestCtx = authn.Into(requestCtx, authn.Claims{Tenant: scope.Tenant, Subject: scope.Subject})
			next.ServeHTTP(w, r.WithContext(requestCtx))
		})
	})
	router.Route("/api/v1", func(r chi.Router) { mountPhotosPrivateSource(r, source) })
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	encoded, _ := json.Marshal(scope)
	pgtest.RunNativeFixture(ctx, t, pgtest.NativeFixture{Module: "../../../../apps/nucleo", Package: "./matrix", Test: "TestPrivatePhotosHTTPSourceAgainstSynapseAndFreshDevice", Marker: "PHOTOS_PG_HTTP_SDK_FRESH_PASS", Timeout: 7 * time.Minute, Env: map[string]string{
		"SYTUE_PHOTOS_SOURCE_FIXTURE": "1", "SYTUE_PHOTOS_SOURCE_FIXTURE_URL": server.URL, "SYTUE_PHOTOS_SOURCE_FIXTURE_SYNAPSE_URL": upstream, "SYTUE_PHOTOS_SOURCE_FIXTURE_SCOPE": string(encoded), "SYTUE_PHOTOS_SOURCE_FIXTURE_ASSET": asset.ID.String(), "SYTUE_PHOTOS_SOURCE_FIXTURE_DRIVE": driveID,
	}})
	state, err := source.State(ctx, scope)
	if err != nil || state.Mode != "frozen" || state.Rows != wantRows || state.Blobs != 2 {
		t.Fatal("retained SQL/S3 source changed", state, err)
	}
	if err := st.GuardarDescripcion(ctx, asset.ID, scope.Subject, "old writer", nil); err == nil {
		t.Fatal("legacy SQL writer escaped frozen state")
	}
	t.Log("PHOTOS_REAL_PG_HTTP_SDK_FRESH_PASS SQL metadata and historical S3 variants retained, opaque plan CAS before freeze, old SQL writers blocked")
}

func photosRetainedS3Server(t *testing.T, bodies map[string][]byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			type object struct {
				Key  string
				Size int
			}
			page := struct {
				XMLName     xml.Name `xml:"ListBucketResult"`
				Name        string
				IsTruncated bool
				Contents    []object
			}{Name: "retained-photos"}
			for key, body := range bodies {
				full := "tenants/acme/" + key
				if strings.HasPrefix(full, r.URL.Query().Get("prefix")) {
					page.Contents = append(page.Contents, object{Key: full, Size: len(body)})
				}
			}
			// MinIO's listing contract is lexicographic, including historical keys.
			if len(page.Contents) == 2 && page.Contents[0].Key > page.Contents[1].Key {
				page.Contents[0], page.Contents[1] = page.Contents[1], page.Contents[0]
			}
			w.Header().Set("Content-Type", "application/xml")
			_ = xml.NewEncoder(w).Encode(page)
			return
		}
		body, ok := bodies[strings.TrimPrefix(r.URL.Path, "/retained-photos/tenants/acme/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("ETag", `"synthetic-retained-body"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("Last-Modified", "Fri, 30 Oct 2026 10:00:00 GMT")
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
