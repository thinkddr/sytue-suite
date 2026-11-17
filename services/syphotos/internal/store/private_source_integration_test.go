// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

type (
	retainedS3Fixture struct {
		mu      sync.Mutex
		bodies  map[string][]byte
		gets    int
		badList string
	}
	retainedS3Object struct {
		Key  string
		Size int64
	}
	retainedS3Page struct {
		XMLName               xml.Name `xml:"ListBucketResult"`
		Name                  string
		IsTruncated           bool
		NextContinuationToken string
		Contents              []retainedS3Object
	}
)

func (f *retainedS3Fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Query().Get("list-type") == "2" {
		prefix, after := r.URL.Query().Get("prefix"), r.URL.Query().Get("start-after")
		keys := []string{}
		for key := range f.bodies {
			full := "tenants/t1/" + key
			if strings.HasPrefix(full, prefix) && full > after {
				keys = append(keys, full)
			}
		}
		slices.Sort(keys)
		index := 0
		if token := r.URL.Query().Get("continuation-token"); token != "" {
			var err error
			index, err = strconv.Atoi(token)
			if err != nil || index < 0 || index > len(keys) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		page := retainedS3Page{Name: "retained-photos"}
		if index < len(keys) {
			key := keys[index]
			page.Contents = []retainedS3Object{{Key: key, Size: int64(len(f.bodies[strings.TrimPrefix(key, "tenants/t1/")]))}}
			index++
		}
		if index < len(keys) {
			page.IsTruncated = true
			page.NextContinuationToken = strconv.Itoa(index)
		}
		switch f.badList {
		case "truncated":
			page.IsTruncated = true
			page.NextContinuationToken = ""
		case "escape":
			page.Contents = []retainedS3Object{{Key: "tenants/other/a/private/file.png", Size: 1}}
			page.IsTruncated = false
			page.NextContinuationToken = ""
		}
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(page)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/retained-photos/tenants/t1/")
	body, ok := f.bodies[key]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code></Error>`))
		return
	}
	sum := sha256.Sum256(body)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Last-Modified", "Fri, 30 Oct 2026 10:00:00 GMT")
	if r.Method == http.MethodHead {
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	f.gets++
	_, _ = w.Write(body)
}

func sourceFixture(t *testing.T) (*store.PrivateSource, *store.Store, *retainedS3Fixture, domain.Activo, storage.Scope) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := ctxDe("t1")
	asset, err := st.Ingerir(ctx, foto("retained.png", "private-source-original", nil))
	if err != nil {
		t.Fatal(err)
	}
	f := &retainedS3Fixture{bodies: map[string][]byte{}}
	for _, recipe := range []string{"old-recipe", "new-recipe"} {
		key, err := (model.Derivada{ActivoID: asset.ID, Huella: recipe, Variante: model.VarianteMiniatura, Formato: "png"}).Clave()
		if err != nil {
			t.Fatal(err)
		}
		f.bodies[key] = []byte("exact retained " + recipe + " thumbnail\x00\xff")
	}
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	objects, err := objectstore.New(objectstore.Config{Endpoint: strings.TrimPrefix(server.URL, "http://"), Region: "eu-test-1", AccessKey: "synthetic", SecretKey: "synthetic", Bucket: "retained-photos"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.NewPrivateSource(st, objects)
	if err != nil {
		t.Fatal(err)
	}
	return source, st, f, asset, storage.Scope{Tenant: "t1", Subject: "u1", Product: "photos"}
}

func TestPrivatePhotosSourceRetainsHistoricalVariantsAndFreezesOnlyExactAcceptedPlan(t *testing.T) {
	source, st, objects, asset, scope := sourceFixture(t)
	ctx := ctxDe("t1")
	epoch := uuid.New()
	preflight, err := source.Preflight(ctx, scope, epoch.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := preflight.Validate(scope, epoch.String()); err != nil || preflight.BackendBlobs != 2 || len(preflight.Rows) != 3 || len(preflight.Backend) != 5 || objects.gets != 2 {
		t.Fatal("incomplete historical variant preflight", preflight.BackendBlobs, len(preflight.Rows), objects.gets, err)
	}
	if _, err := model.NewSnapshot(scope, preflight.Rows); err != nil {
		t.Fatal("variant relationships lost", err)
	}
	if _, err := source.Begin(ctx, scope, epoch); err == nil {
		t.Fatal("source froze before a durable encrypted plan reference")
	}
	link := storage.Reference{RoomID: "!photos:test", EventID: "$accepted-plan"}
	if _, err := source.AcceptPlan(ctx, scope, epoch.String(), uuid.NewString(), link); err == nil {
		t.Fatal("wrong preflight ticket accepted")
	}
	if _, err := source.AcceptPlan(ctx, scope, epoch.String(), preflight.CAS, link); err != nil {
		t.Fatal(err)
	}
	routing, err := source.PlanRouting(ctx, scope)
	if err != nil || routing.Plan == nil || routing.Plan.Link != link || routing.Plan.Epoch != epoch.String() {
		t.Fatal("opaque plan reference lost", err)
	}
	before, err := source.State(ctx, scope)
	if err != nil || before.Mode != "legacy" {
		t.Fatal("plan acceptance silently froze source", err)
	}
	state, err := source.Begin(ctx, scope, epoch)
	if err != nil || state.Mode != "frozen" || state.Rows != 5 || state.Blobs != 2 {
		t.Fatal("backend counts changed or source did not freeze", state, err)
	}
	page, err := source.Page(ctx, scope, epoch, storage.LegacySourceCursor{}, storage.MaxLegacySourceRows)
	if err != nil || !slices.Equal(page.Rows, preflight.Backend) {
		t.Fatal("retained descriptor inventory changed", err)
	}
	for _, row := range page.Rows {
		descriptor, reader, err := source.Open(ctx, scope, epoch, row.Kind, row.ID)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		closeErr := reader.Close()
		sum := sha256.Sum256(body)
		if err != nil || closeErr != nil || descriptor != row || int64(len(body)) != row.Size || hex.EncodeToString(sum[:]) != row.SHA256 {
			t.Fatal("retained row bytes changed", row.Kind, err, closeErr)
		}
	}
	if err := st.GuardarDescripcion(ctx, asset.ID, "u1", "changed", nil); !privatePhotoBlocked(err) {
		t.Fatal("frozen legacy metadata remained writable", err)
	}
	if _, _, err := source.Open(ctx, scope, epoch, "original", asset.ID.String()); err == nil {
		t.Fatal("private original traversed legacy HTTP source")
	}
	wrong := scope
	wrong.Subject = "other"
	if _, _, err := source.Open(ctx, wrong, epoch, "variant", preflight.Backend[len(preflight.Backend)-1].ID); err == nil {
		t.Fatal("foreign owner opened retained variant")
	}
}

func TestPrivatePhotosSourceCASRejectsMetadataAndVariantChangesBeforeFreeze(t *testing.T) {
	for _, change := range []string{"metadata before accept", "variant before accept", "variant after accept", "new historical variant"} {
		t.Run(change, func(t *testing.T) {
			source, st, objects, asset, scope := sourceFixture(t)
			ctx := ctxDe("t1")
			epoch := uuid.New()
			preflight, err := source.Preflight(ctx, scope, epoch.String())
			if err != nil {
				t.Fatal(err)
			}
			link := storage.Reference{RoomID: "!photos:test", EventID: "$accepted-plan"}
			if change == "variant after accept" {
				if _, err := source.AcceptPlan(ctx, scope, epoch.String(), preflight.CAS, link); err != nil {
					t.Fatal(err)
				}
			}
			if change == "metadata before accept" {
				if err := st.GuardarDescripcion(ctx, asset.ID, "u1", "changed", nil); err != nil {
					t.Fatal(err)
				}
			} else {
				objects.mu.Lock()
				if change == "new historical variant" {
					key, _ := (model.Derivada{ActivoID: asset.ID, Huella: "forgotten-old-recipe", Variante: model.VarianteGrande, Formato: "png"}).Clave()
					objects.bodies[key] = []byte("retained historical image")
				} else {
					for key, body := range objects.bodies {
						objects.bodies[key] = bytes.Repeat([]byte("x"), len(body))
						break
					}
				}
				objects.mu.Unlock()
			}
			if change == "variant after accept" {
				if _, err := source.Begin(ctx, scope, epoch); !errors.Is(err, store.ErrPrivateSource) {
					t.Fatal("changed S3 body froze source", err)
				}
			} else if _, err := source.AcceptPlan(ctx, scope, epoch.String(), preflight.CAS, link); !errors.Is(err, store.ErrPrivateSource) {
				t.Fatal("stale inventory CAS accepted", err)
			}
			state, err := source.State(ctx, scope)
			if err != nil || state.Mode != "legacy" {
				t.Fatal("rejected CAS partially froze source", state, err)
			}
		})
	}
}

func TestPrivatePhotosSourceRejectsSharedOrIncompleteInventoriesBeforeFreeze(t *testing.T) {
	for _, change := range []string{"shared", "truncated", "escape", "unknown variant"} {
		t.Run(change, func(t *testing.T) {
			source, st, objects, asset, scope := sourceFixture(t)
			ctx := ctxDe("t1")
			switch change {
			case "shared":
				album, err := st.CrearAlbum(ctx, "u1", "shared")
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err := st.CompartirConPersona(ctx, album.ID, "u1", "other", false); err != nil {
					t.Fatal(err)
				}
			case "unknown variant":
				objects.bodies["a/"+asset.ID.String()+"/old-recipe/unknown.png"] = []byte("retained unsupported variant")
			default:
				objects.badList = change
			}
			_, preflightErr := source.Preflight(ctx, scope, uuid.NewString())
			if preflightErr == nil {
				t.Fatal("incomplete source preflight accepted")
			}
			if change == "shared" && !errors.Is(preflightErr, model.ErrShared) {
				t.Fatal("share rejected for wrong cause", preflightErr)
			}
			if change == "unknown variant" && !errors.Is(preflightErr, store.ErrPrivateSource) {
				t.Fatal("unsupported variant rejected for wrong cause", preflightErr)
			}
			state, err := source.State(ctx, scope)
			if err != nil || state.Mode != "legacy" {
				t.Fatal("rejected preflight partially froze source", err)
			}
			if state.Shared != (change == "shared") {
				t.Fatal("legacy routing hid collaboration", state.Shared)
			}
			if change == "shared" && objects.gets != 0 {
				t.Fatal("shared scope opened S3 bodies before rejection")
			}
		})
	}
}
