// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type photosRetainedSourceFixture struct {
	notesRetainedSourceFixture
	blobs int64
	mu    sync.Mutex
}

func (s *photosRetainedSourceFixture) opensCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opens
}

func (s *photosRetainedSourceFixture) beginsCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.begins
}

func (s *photosRetainedSourceFixture) Begin(ctx context.Context, scope storage.Scope, epoch string) (privatePhotos.SnapshotState, error) {
	state, err := s.notesRetainedSourceFixture.Begin(ctx, scope, epoch)
	state.Blobs = s.blobs
	s.state = state
	return state, err
}

func (s *photosRetainedSourceFixture) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := storage.ValidateRequestScope(r.Header, s.state.Scope); err != nil {
			w.WriteHeader(http.StatusConflict)
			return
		}
		ctx := r.Context()
		if strings.Contains(r.URL.Path, "/source/") {
			key := strings.SplitN(r.URL.Path, "/source/", 2)[1]
			raw, ok := s.rows[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			s.opens++
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
			w.Header().Set("X-Sytue-Source-SHA256", digestBytes(raw))
			_, _ = w.Write(raw)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/rows") {
			page, err := s.Page(ctx, s.state.Scope, s.state.Epoch, storage.LegacySourceCursor{})
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(page)
			return
		}
		if r.Method == http.MethodPost {
			var request struct {
				Epoch string `json:"epoch"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if _, err := s.Begin(ctx, s.state.Scope, request.Epoch); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusConflict)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(s.state)
	})
}

type lostPhotosBinaryIndexACK struct {
	*Client
	drop     bool
	accepted storage.Reference
}

func (s *lostPhotosBinaryIndexACK) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	version, err := s.Client.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
	var summary struct {
		Binary bool `json:"binary"`
	}
	if err == nil && s.drop && json.Unmarshal(commit.Metadata, &summary) == nil && summary.Binary {
		s.drop = false
		s.accepted = version.Reference
		return storage.ObjectVersion{}, errors.New("synthetic lost accepted Photos binary index ACK")
	}
	return version, err
}

func TestPrivatePhotosAgainstSynapseImportsMediaAndRecoversFreshDevice(t *testing.T) {
	exercisePrivatePhotosStorage(t, false)
}

func TestPrivatePhotosRecipeAgainstSynapseAtomicPublicationBoundaries(t *testing.T) {
	exercisePrivatePhotosStorage(t, true)
}

func exercisePrivatePhotosStorage(t *testing.T, boundaries bool) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_PHOTOS") != "1" {
		t.Skip("requires isolated disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Minute)
	defer cancel()
	upstream, err := url.Parse(os.Getenv("SYTUE_MATRIX_PRIVATE_PHOTOS_URL"))
	if err != nil || upstream.Host == "" {
		t.Fatal("disposable Synapse URL required")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "photos"}
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	opaque := &fileGatewayFixture{product: "photos"}
	blobServer := httptest.NewServer(opaque)
	t.Cleanup(blobServer.Close)
	gateway := api.NewDrive(blobServer.URL, nil, blobServer.Client())
	photoID := uuid.NewString()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 16, 12))); err != nil {
		t.Fatal(err)
	}
	thumbnail := bytes.Clone(encoded.Bytes())
	original := append(bytes.Clone(encoded.Bytes()), []byte(strings.Repeat("retained Photos original private sentinel", 32768))...)
	mediaOriginal := model.MediaRow{ID: photoID, Tenant: scope.Tenant, Owner: scope.Subject, AssetID: photoID, Role: "original", Size: int64(len(original)), SHA256: digestBytes(original), ContentType: "image/png"}
	recipe := model.Receta{Version: 1, Pasos: []model.Paso{{Tipo: model.PasoRecorte, Params: map[string]float64{"x": 0, "y": 0, "ancho": .5, "alto": 1}}}}
	fingerprint, err := recipe.Huella()
	if err != nil {
		t.Fatal(err)
	}
	derived := model.Derivada{ActivoID: uuid.MustParse(photoID), Huella: fingerprint, Variante: model.VarianteMiniatura, Formato: "png"}
	key, err := derived.Clave()
	if err != nil {
		t.Fatal(err)
	}
	variantID := digestBytes([]byte(key))
	mediaVariant := model.MediaRow{ID: variantID, Tenant: scope.Tenant, Owner: scope.Subject, AssetID: photoID, Role: "variant", Size: int64(len(thumbnail)), SHA256: digestBytes(thumbnail), ContentType: "image/png", Fingerprint: fingerprint, Variant: model.VarianteMiniatura, Format: "png"}
	asset := []byte(`{"id":"` + photoID + `","tenant":"acme","owner_sub":"` + scope.Subject + `","drive_file_id":"retained-original-drive-id","name":"retained.png","media_type":"image/png","bytes":` + strconv.Itoa(len(original)) + `,"width":16,"height":12,"orientation":6,"uploaded_at":"2026-10-30T10:00:00Z","future_integer":900719925474099312345}`)
	source := &photosRetainedSourceFixture{notesRetainedSourceFixture: notesRetainedSourceFixture{state: privatePhotos.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, rows: map[string][]byte{"asset/" + photoID: asset, "original/" + photoID: original, "variant/" + variantID: thumbnail}}, blobs: 2}
	editID := uuid.NewString()
	editRaw, err := json.Marshal(model.EditRow{ID: editID, Tenant: scope.Tenant, Owner: scope.Subject, AssetID: photoID, Recipe: recipe, Fingerprint: fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	var editFields map[string]json.RawMessage
	_ = json.Unmarshal(editRaw, &editFields)
	var recipeFields map[string]json.RawMessage
	_ = json.Unmarshal(editFields["recipe"], &recipeFields)
	var steps []map[string]json.RawMessage
	_ = json.Unmarshal(recipeFields["pasos"], &steps)
	steps[0]["future_step_integer"] = json.RawMessage("900719925474099312345")
	recipeFields["pasos"], _ = json.Marshal(steps)
	recipeFields["future_recipe_integer"] = json.RawMessage("900719925474099312345")
	editFields["recipe"], _ = json.Marshal(recipeFields)
	editFields["future_integer"] = json.RawMessage("900719925474099312345")
	editRaw, err = json.Marshal(editFields)
	if err != nil {
		t.Fatal(err)
	}
	source.rows["edit/"+editID] = bytes.Clone(editRaw)
	source.rows["media/"+photoID], _ = json.Marshal(mediaOriginal)
	source.rows["media/"+variantID], _ = json.Marshal(mediaVariant)
	server := httptest.NewServer(source.handler(t))
	t.Cleanup(server.Close)
	legacy := api.NewPhotos(server.URL, nil, server.Client())
	port, err := privatePhotos.NewHTTPSource(legacy)
	if err != nil {
		t.Fatal(err)
	}
	newJournal := func() *offline.Store {
		t.Helper()
		journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
		if err != nil {
			t.Fatal(err)
		}
		return journal
	}
	open := func(crypto privatePhotos.Crypto, journal *offline.Store) *privatePhotos.Repository {
		t.Helper()
		repository, err := privatePhotos.New(privatePhotos.Config{Context: ctx, Scope: scope, Crypto: crypto, Gateway: gateway, Journal: journal, Source: port})
		if err != nil {
			t.Fatal(err)
		}
		return repository
	}
	journal := newJournal()
	lost := &lostPhotosBinaryIndexACK{Client: device.Client, drop: true}
	first := open(lost, journal)
	defer first.Close()
	if err := first.Initialize(ctx); !errors.Is(err, privatePhotos.ErrNotReady) || source.beginsCount() != 0 || source.opensCount() != 0 {
		t.Fatal("ordinary initialization imported source", err)
	}
	if err := first.ImportLegacySnapshot(ctx); err == nil || lost.drop || !lost.accepted.Valid() {
		t.Fatal("binary accepted ACK was not lost", err)
	}
	if err := device.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	fresh := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), device)
	second := open(fresh, newJournal())
	defer second.Close()
	if err := second.Initialize(ctx); !errors.Is(err, privatePhotos.ErrNotReady) {
		t.Fatal("partial media import activated", err)
	}
	before := source.opensCount()
	if err := second.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal("fresh explicit import handoff", err)
	}
	if source.opensCount() != before+1 {
		t.Fatal("accepted original or metadata reimported", source.opensCount(), before)
	}
	if err := first.Initialize(ctx); err != nil {
		t.Fatal("original device WAL reconciliation", err)
	}
	if count, err := journal.Count("photos-private-stage-v1"); err != nil || count != 0 {
		t.Fatal("acknowledged media spool retained", count, err)
	}
	facade, err := legacy.WithPrivateData(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := facade.SetFavorite(ctx, photoID, true); err != nil {
		t.Fatal(err)
	}
	if err := facade.SetDescription(ctx, photoID, "private description sentinel", []string{"private tag"}); err != nil {
		t.Fatal(err)
	}
	if err := facade.Trash(ctx, photoID); err != nil {
		t.Fatal(err)
	}
	if values, err := facade.Papelera(ctx); err != nil || len(values.Items) != 1 {
		t.Fatal("private trash", err)
	}
	if err := facade.Restaurar(ctx, photoID); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.ShareAlbum(ctx, uuid.NewString()); !errors.Is(err, api.ErrPrivatePhotosOperation) {
		t.Fatal("unsupported operation escaped to HTTP", err)
	}
	reads := source.opensCount()
	if err := first.RefreshPrivateData(ctx); err != nil {
		t.Fatal("explicit current catalog refresh from another SDK device", err)
	}
	photos, _, err := first.Timeline(ctx, "", 10, true, "private description")
	if err != nil || len(photos) != 1 || !photos[0].Favorite {
		t.Fatal("private metadata query", err)
	}
	raw, err := first.ReadRaw(ctx, "asset", photoID)
	if err != nil || !bytes.Contains(raw, []byte(`900719925474099312345`)) {
		t.Fatal("private edits changed retained numbers", err)
	}
	clear(raw)
	var received bytes.Buffer
	if err := second.Original(ctx, photoID, &received); err != nil || !bytes.Equal(received.Bytes(), original) {
		t.Fatal("original bytes changed", err)
	}
	received.Reset()
	if err := second.ReadVariant(ctx, variantID, &received); err != nil || !bytes.Equal(received.Bytes(), thumbnail) {
		t.Fatal("thumbnail bytes changed", err)
	}
	mediaBody, err := facade.VariantDe(ctx, photoID, string(model.VarianteMiniatura), fingerprint)
	if err != nil {
		t.Fatal("private facade could not open retained thumbnail", err)
	}
	mediaBytes, err := io.ReadAll(mediaBody)
	_ = mediaBody.Close()
	if err != nil || !bytes.Equal(mediaBytes, thumbnail) {
		t.Fatal("private thumbnail stream changed original bytes", err)
	}
	clear(mediaBytes)
	mediaBody, err = facade.Variant(ctx, photoID, string(model.VarianteMiniatura))
	if err != nil {
		t.Fatal("private facade could not resolve current recipe", err)
	}
	mediaBytes, err = io.ReadAll(mediaBody)
	_ = mediaBody.Close()
	if err != nil || !bytes.Equal(mediaBytes, thumbnail) {
		t.Fatal("current private thumbnail changed", err)
	}
	clear(mediaBytes)
	if _, err := facade.VariantDe(ctx, photoID, string(model.VarianteMiniatura), strings.Repeat("a", 64)); !errors.Is(err, privatePhotos.ErrNotFound) {
		t.Fatal("missing private thumbnail did not remain unavailable", err)
	}
	preview, err := facade.VariantDe(ctx, photoID, api.PhotoPreview, api.RecetaVacia().Huella())
	if err != nil {
		t.Fatal("missing empty-recipe preview did not use the authorized original", err)
	}
	previewRaw, err := io.ReadAll(preview)
	_ = preview.Close()
	if err != nil {
		t.Fatal("authorized original preview read", err)
	}
	previewImage, _, err := image.Decode(bytes.NewReader(previewRaw))
	clear(previewRaw)
	if err != nil || previewImage.Bounds().Dx() != 12 || previewImage.Bounds().Dy() != 16 {
		t.Fatal("original preview ignored EXIF or applied the current crop", err)
	}
	if _, err := facade.VariantDe(ctx, photoID, api.PhotoPreview, strings.Repeat("a", 12)); !errors.Is(err, privatePhotos.ErrNotFound) {
		t.Fatal("an arbitrary fingerprint triggered original rendering", err)
	}
	for _, format := range []string{"png", "jpeg"} {
		exported, err := facade.Export(ctx, photoID, format)
		if err != nil {
			t.Fatal("private export could not open", format, err)
		}
		raw, err := io.ReadAll(exported)
		_ = exported.Close()
		if err != nil {
			t.Fatal("private export failed", format, err)
		}
		decoded, kind, err := image.Decode(bytes.NewReader(raw))
		clear(raw)
		if err != nil || kind != format || decoded.Bounds().Dx() != 6 || decoded.Bounds().Dy() != 16 {
			t.Fatal("private export lost EXIF or retained crop", format, kind, err)
		}
		if format == "jpeg" {
			red, green, blue, _ := decoded.At(0, 0).RGBA()
			if red < 65000 || green < 65000 || blue < 65000 {
				t.Fatal("JPEG export lost the white transparency background")
			}
		}
	}
	retainedRecipe, err := second.ReadRaw(ctx, "edit", editID)
	if err != nil || !bytes.Equal(retainedRecipe, editRaw) {
		t.Fatal("private export rewrote the retained recipe", err)
	}
	clear(retainedRecipe)
	received.Reset()
	if err := second.Original(ctx, photoID, &received); err != nil || !bytes.Equal(received.Bytes(), original) {
		t.Fatal("private export rewrote original bytes", err)
	}
	if _, err := facade.Export(ctx, photoID, "gif"); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("unsupported private export escaped its format guard", err)
	}
	if source.opensCount() != reads {
		t.Fatal("private reads reimported plaintext source")
	}
	if !boundaries {
		exercisePrivateRecipeSave(ctx, t, first, second, facade, photoID, fingerprint, original, thumbnail, variantID, editID)
	}
	snapshot, err := storage.LoadCatalog(ctx, fresh, scope)
	if err != nil {
		t.Fatal(err)
	}
	for ref := range snapshot.Entries {
		stored, err := device.Matrix.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
		if err != nil || stored.Type != event.EventEncrypted {
			t.Fatal("plaintext Photos index on Synapse", err)
		}
	}
	for _, body := range opaque.data {
		if bytes.Contains(body, []byte("Photos original private sentinel")) || bytes.Contains(body, []byte("private description sentinel")) {
			t.Fatal("opaque gateway received clear content")
		}
	}
	if boundaries {
		first.Close()
		second.Close()
		exercisePrivateRecipeBoundaries(ctx, t, photoRecipeBoundaryFixture{device: device, peer: fresh, base: proxy.URL, httpClient: proxy.Client(), asset: photoID, scope: scope, source: source, open: open, journal: newJournal})
	}
	t.Log("real SDK/Synapse: exact original+thumbnail, authenticated catalog+media receipts, lost binary index ACK, fresh device with empty journal, no plaintext reimport, private favorite/description/trash/restore; production retained source and UI remain unactivated")
}

func exercisePrivateRecipeSave(ctx context.Context, t *testing.T, reader, writer *privatePhotos.Repository, facade *api.Photos, asset, previous string, original, thumbnail []byte, variantID, editID string) {
	t.Helper()
	recipe, err := writer.Recipe(ctx, asset)
	if err != nil {
		t.Fatal("private recipe read", err)
	}
	recipe.Pasos = append(recipe.Pasos, model.Paso{Tipo: model.PasoGiro, Params: map[string]float64{"grados": 90}})
	fingerprint, err := writer.SaveRecipe(ctx, asset, recipe, previous)
	if err != nil {
		t.Fatal("private recipe save", err)
	}
	if fingerprint == previous {
		t.Fatal("saved recipe reused the preceding fingerprint")
	}
	edited, err := writer.ReadRaw(ctx, "edit", editID)
	if err != nil {
		t.Fatal(err)
	}
	var edit map[string]json.RawMessage
	_ = json.Unmarshal(edited, &edit)
	clear(edited)
	var savedRecipe map[string]json.RawMessage
	_ = json.Unmarshal(edit["recipe"], &savedRecipe)
	var savedSteps []map[string]json.RawMessage
	_ = json.Unmarshal(savedRecipe["pasos"], &savedSteps)
	integer := []byte("900719925474099312345")
	if !bytes.Equal(edit["future_integer"], integer) || !bytes.Equal(savedRecipe["future_recipe_integer"], integer) || len(savedSteps) < 1 || !bytes.Equal(savedSteps[0]["future_step_integer"], integer) {
		t.Fatal("private recipe save lost unknown fields or numeric precision")
	}

	if err := reader.RefreshPrivateData(ctx); err != nil {
		t.Fatal("reader refresh after recipe save", err)
	}
	current, err := reader.Recipe(ctx, asset)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := current.Huella()
	if err != nil || actual != fingerprint {
		t.Fatal("peer did not select committed recipe", err)
	}
	for _, variant := range model.Variantes() {
		body, err := facade.VariantDe(ctx, asset, string(variant), fingerprint)
		if err != nil {
			t.Fatal("saved variant open", variant, err)
		}
		raw, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			t.Fatal("saved variant read", variant, err)
		}
		img, kind, err := image.Decode(bytes.NewReader(raw))
		clear(raw)
		if err != nil || kind != "png" || img.Bounds().Dx() != 16 || img.Bounds().Dy() != 6 {
			t.Fatal("saved rendition lost EXIF/crop/rotation", variant, kind, err)
		}
	}
	body, err := facade.Export(ctx, asset, "png")
	if err != nil {
		t.Fatal("edited export open", err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	clear(raw)
	if err != nil || img.Bounds().Dx() != 16 || img.Bounds().Dy() != 6 {
		t.Fatal("edited export ignored the committed recipe", err)
	}
	var retained bytes.Buffer
	if err := writer.Original(ctx, asset, &retained); err != nil || !bytes.Equal(retained.Bytes(), original) {
		t.Fatal("recipe changed original bytes", err)
	}
	retained.Reset()
	if err := writer.ReadVariant(ctx, variantID, &retained); err != nil || !bytes.Equal(retained.Bytes(), thumbnail) {
		t.Fatal("recipe changed a historical derivative", err)
	}
	if _, err := writer.SaveRecipe(ctx, asset, model.RecetaVacia(), previous); !errors.Is(err, privatePhotos.ErrConflict) {
		t.Fatal("stale recipe base overwrote committed editing", err)
	}
}
