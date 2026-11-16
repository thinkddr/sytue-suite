// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/exif"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

func ingestFixtureScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
}

func ingestFixtureTarget() storage.PublicationTarget {
	return storage.PublicationTarget{RoomID: "!private-photos", DeviceID: "INGESTDEVICE"}
}

func ingestFixtureLink(scope storage.Scope, asset string, size int64, digest string) records.RetainedFile {
	return records.RetainedFile{
		Version: 1, Scope: scope, Target: ingestFixtureTarget(),
		Row:      records.SnapshotRow{Kind: "original", ID: asset, Size: size, SHA256: digest},
		Revision: "retained-revision-1",
		Link:     storage.Reference{RoomID: "!private-photos", EventID: "$retained-link"},
	}
}

// ingestFixtureRow builds the exact media row prepareIngestMedia publishes.
func ingestFixtureRow(t *testing.T, scope storage.Scope, origin ingestOrigin, file records.RetainedFile) []byte {
	t.Helper()
	base, err := json.Marshal(model.MediaRow{ID: origin.Asset, Tenant: scope.Tenant, Owner: scope.Subject, AssetID: origin.Asset, Role: "original", Size: file.Row.Size, SHA256: file.Row.SHA256, ContentType: origin.Drive.MediaType})
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(base, &fields) != nil {
		t.Fatal("fixture media row")
	}
	if fields[ingestOriginalField], err = json.Marshal(file); err != nil {
		t.Fatal(err)
	}
	if fields[ingestIntentField], err = json.Marshal(origin.intent()); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestIngestIntentBuildsAValidAssetRow(t *testing.T) {
	scope := ingestFixtureScope()
	taken := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	lat, lon := 41.64, -0.88
	origin := ingestOrigin{
		Drive: ingestFile{ID: "drive-file", Name: "playa.png", MediaType: "image/png", Bytes: 42, UpdatedAt: taken},
		Exif:  exif.Metadatos{Orientacion: 6, TomadaEn: &taken, Camara: "Cámara de prueba", Objetivo: "Objetivo", Lat: &lat, Lon: &lon},
		Width: 6, Height: 4, Captured: taken,
	}
	intent := origin.intent()
	asset := intent.assetRow(uuid.NewString(), scope)
	raw, err := json.Marshal(asset)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.ValidateRow(scope, model.Row{Kind: "asset", ID: asset.ID, JSON: raw}); err != nil {
		t.Fatal("ingest intent did not produce a valid asset row", err)
	}
	if asset.Orientation != 6 || asset.TakenAt == nil || !asset.TakenAt.Equal(taken) || asset.Latitude == nil || *asset.Latitude != lat || asset.DriveID != "drive-file" || asset.Name != "playa.png" || !asset.UploadedAt.Equal(taken) || asset.CreatedAt.IsZero() {
		t.Fatal("ingest intent lost captured metadata", asset)
	}
}

func TestIngestDimensionsOnlyAcceptsWhatRenderReopens(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for i := range picture.Pix {
		picture.Pix[i] = byte(i)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	width, height, err := ingestDimensions(encoded.Bytes())
	if err != nil || width != 32 || height != 16 {
		t.Fatal("ingest did not measure a decodable original", width, height, err)
	}
	if _, _, err := ingestDimensions([]byte("ni PNG ni JPEG: nada que reabrir")); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("undecodable original was not rejected", err)
	}
}

func TestIngestedOriginalDecodesItsLinkAndIntent(t *testing.T) {
	scope := ingestFixtureScope()
	asset := uuid.NewString()
	taken := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	origin := ingestOrigin{Asset: asset, Drive: ingestFile{ID: "drive-file", Name: "playa.png", MediaType: "image/png", Bytes: 42, UpdatedAt: taken}, Exif: exif.Metadatos{Orientacion: 1, TomadaEn: &taken}, Width: 6, Height: 4, Captured: taken}
	file := ingestFixtureLink(scope, asset, 42, planBytesHash([]byte("cuerpo-original")))
	raw := ingestFixtureRow(t, scope, origin, file)
	row, intent, linked, err := ingestedOriginal(raw, scope, asset)
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != asset || row.Role != "original" || row.Size != 42 || row.ContentType != "image/png" || intent.DriveID != "drive-file" || linked.Link.EventID != "$retained-link" || linked.Row.ID != asset {
		t.Fatal("ingested row decoded wrong", row, intent, linked)
	}
	// Sin su enlace la fila es inservible: el original es indescubrible.
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields) != nil {
		t.Fatal("fixture row")
	}
	delete(fields, ingestOriginalField)
	broken, _ := json.Marshal(fields)
	if _, _, _, err := ingestedOriginal(broken, scope, asset); !errors.Is(err, ErrSource) {
		t.Fatal("row without its retained link decoded", err)
	}
	// El descriptor del enlace y el de la fila media tienen que ser el mismo.
	fields2 := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields2) != nil {
		t.Fatal("fixture row")
	}
	changed := file
	changed.Row.SHA256 = planBytesHash([]byte("otro-cuerpo"))
	fields2[ingestOriginalField], _ = json.Marshal(changed)
	mismatch, _ := json.Marshal(fields2)
	if _, _, _, err := ingestedOriginal(mismatch, scope, asset); !errors.Is(err, ErrSource) {
		t.Fatal("link descriptor diverged from the media row", err)
	}
}

func TestIngestPairValidatesCapturedAgainstPrepared(t *testing.T) {
	scope := ingestFixtureScope()
	asset := uuid.NewString()
	taken := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	origin := ingestOrigin{
		Version: 1, Scope: scope, Target: ingestFixtureTarget(), Operation: ingestMediaOperation, Asset: asset,
		Drive: ingestFile{ID: "drive-file", Name: "playa.png", MediaType: "image/png", Bytes: 42, UpdatedAt: taken},
		Pin:   drive.FilePin{Scope: storage.Scope{Tenant: scope.Tenant, Subject: scope.Subject, Product: "drive"}, SourceKind: "file", SourceID: "drive-file", IndexRef: storage.Reference{RoomID: "!drive", EventID: "$index"}, ObjectID: "object", ContentRevision: "revision", SizeBytes: 42, SHA256: planBytesHash([]byte("cuerpo-original"))},
		Exif:  exif.Metadatos{Orientacion: 6, TomadaEn: &taken}, Width: 6, Height: 4, Captured: taken,
	}
	file := ingestFixtureLink(scope, asset, 42, origin.Pin.SHA256)
	raw := ingestFixtureRow(t, scope, origin, file)
	captured, err := json.Marshal(origin)
	if err != nil {
		t.Fatal(err)
	}
	r := &Repository{scope: scope}
	if _, _, _, _, err := r.ingestPair(raw, captured, ingestFixtureTarget()); err != nil {
		t.Fatal("consistent ingest pair rejected", err)
	}
	tamper := func(mutate func(*ingestOrigin)) error {
		t.Helper()
		var changed ingestOrigin
		if json.Unmarshal(captured, &changed) != nil {
			t.Fatal("captured origin")
		}
		mutate(&changed)
		edited, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, _, err = r.ingestPair(raw, edited, ingestFixtureTarget())
		return err
	}
	if err := tamper(func(o *ingestOrigin) { o.Drive.ID = "otro-fichero" }); !errors.Is(err, ErrSource) {
		t.Fatal("changed drive id passed", err)
	}
	if err := tamper(func(o *ingestOrigin) { o.Pin.SizeBytes = 41 }); !errors.Is(err, ErrSource) {
		t.Fatal("pin without the drive size passed", err)
	}
	if err := tamper(func(o *ingestOrigin) { o.Pin.Scope.Subject = "bob" }); !errors.Is(err, ErrSource) {
		t.Fatal("foreign subject pin passed", err)
	}
	if err := tamper(func(o *ingestOrigin) { o.Exif.Orientacion = 1 }); !errors.Is(err, ErrSource) {
		t.Fatal("changed orientation passed", err)
	}
	later := taken.Add(time.Hour)
	if err := tamper(func(o *ingestOrigin) { o.Exif.TomadaEn = &later }); !errors.Is(err, ErrSource) {
		t.Fatal("changed capture date passed", err)
	}
}

type ingestCatalogFixture struct {
	entries []records.Entry
	rows    map[string][]byte
	links   map[records.RetainedFile]bool
}

func (f *ingestCatalogFixture) Entries() ([]records.Entry, error) { return f.entries, nil }
func (f *ingestCatalogFixture) ReadJSON(_ context.Context, entry records.Entry) ([]byte, error) {
	return f.rows[entry.Kind+"\x00"+entry.ID], nil
}

func (f *ingestCatalogFixture) ValidateRetainedFile(_ context.Context, file records.RetainedFile) error {
	if !f.links[file] {
		return ErrSource
	}
	return nil
}

func TestIngestMediaRowStaysDormantUntilItsAssetLands(t *testing.T) {
	scope := ingestFixtureScope()
	asset := uuid.NewString()
	taken := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	origin := ingestOrigin{Asset: asset, Drive: ingestFile{ID: "drive-file", Name: "playa.png", MediaType: "image/png", Bytes: 42, UpdatedAt: taken}, Exif: exif.Metadatos{Orientacion: 1}, Width: 6, Height: 4, Captured: taken}
	file := ingestFixtureLink(scope, asset, 42, planBytesHash([]byte("cuerpo-original")))
	raw := ingestFixtureRow(t, scope, origin, file)
	catalog := &ingestCatalogFixture{
		entries: []records.Entry{{Kind: "media", ID: asset, State: "live"}},
		rows:    map[string][]byte{"media\x00" + asset: raw},
		links:   map[records.RetainedFile]bool{file: true},
	}
	// Sin fila asset: la ingesta está a medias y no entra en el grafo.
	visible, inject, _, err := ingestMediaRow(t.Context(), catalog, raw, scope, map[string]bool{})
	if err != nil || visible || inject {
		t.Fatal("half ingest became visible without its asset", visible, inject, err)
	}
	// Con su fila asset: visible, con el descriptor inyectado del enlace.
	live := map[string]bool{asset: true}
	visible, inject, linked, err := ingestMediaRow(t.Context(), catalog, raw, scope, live)
	if err != nil || !visible || !inject || linked != file {
		t.Fatal("completed ingest did not surface its retained original", visible, inject, err)
	}
	// Fila del import (sin enlace): su binario ya es del catálogo.
	plain, err := json.Marshal(model.MediaRow{ID: asset, Tenant: scope.Tenant, Owner: scope.Subject, AssetID: asset, Role: "original", Size: 42, SHA256: file.Row.SHA256, ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	visible, inject, _, err = ingestMediaRow(t.Context(), catalog, plain, scope, live)
	if err != nil || !visible || inject {
		t.Fatal("import media row was treated as ingested", visible, inject, err)
	}
	// Un enlace que no valida es corrupción, no una ingesta dormida.
	broken := catalog
	broken.links = map[records.RetainedFile]bool{}
	if _, _, _, err := ingestMediaRow(t.Context(), broken, raw, scope, live); !errors.Is(err, ErrSource) {
		t.Fatal("unvalidated retained link entered the snapshot", err)
	}
}
