// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivatePhotosHTTPSourceAgainstSynapseAndFreshDevice(t *testing.T) {
	if os.Getenv("SYTUE_PHOTOS_SOURCE_FIXTURE") != "1" {
		t.Skip("requires real PostgreSQL parent source")
	}
	upstream := notesSourceFixtureURL(t, "SYTUE_PHOTOS_SOURCE_FIXTURE_SYNAPSE_URL")
	base := notesSourceFixtureURL(t, "SYTUE_PHOTOS_SOURCE_FIXTURE_URL")
	var scope storage.Scope
	if json.Unmarshal([]byte(os.Getenv("SYTUE_PHOTOS_SOURCE_FIXTURE_SCOPE")), &scope) != nil || !scope.Valid() || scope.Product != "photos" {
		t.Fatal("invalid fixture scope")
	}
	asset, driveID := os.Getenv("SYTUE_PHOTOS_SOURCE_FIXTURE_ASSET"), os.Getenv("SYTUE_PHOTOS_SOURCE_FIXTURE_DRIVE")
	if _, err := uuid.Parse(asset); err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(driveID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(proxy.Close)
	driveScope := scope
	driveScope.Product = "drive"
	driveDevice := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), driveScope, storage.LegacyIdentity)
	photoDevice := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	newJournal := func() *offline.Store {
		t.Helper()
		s, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	driveSource := syntheticIdentitySource(t, driveScope)
	oldID := driveSource.File
	for i, row := range driveSource.Rows {
		row.Payload = bytes.ReplaceAll(row.Payload, []byte(`"`+oldID+`"`), []byte(`"`+driveID+`"`))
		if row.Kind == "file" && row.ID == oldID {
			row.ID = driveID
		}
		driveSource.Rows[i] = row
	}
	driveSource.Blobs["file/"+driveID] = driveSource.Blobs["file/"+oldID]
	delete(driveSource.Blobs, "file/"+oldID)
	driveSource.File = driveID
	slices.SortFunc(driveSource.Rows, func(a, b storage.LegacySourceRow) int {
		if order := strings.Compare(a.Kind, b.Kind); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	original := bytes.Clone(driveSource.Blobs["file/"+driveID])
	driveObjects := &fileGatewayFixture{product: "drive", policyScope: driveScope}
	driveServer := httptest.NewServer(driveObjects)
	t.Cleanup(driveServer.Close)
	driveGateway := api.NewDrive(driveServer.URL, nil, driveServer.Client())
	driveRepository, err := privateDrive.New(privateDrive.Config{Context: ctx, Scope: driveScope, Crypto: driveDevice.Client, Gateway: driveGateway, Journal: newJournal(), Source: driveSource})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(driveRepository.Close)
	if err := driveRepository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	pins, err := privatePhotos.NewDriveOriginalPins(driveRepository)
	if err != nil {
		t.Fatal(err)
	}
	photoObjects := &fileGatewayFixture{product: "photos", policyScope: scope}
	photoServer := httptest.NewServer(photoObjects)
	t.Cleanup(photoServer.Close)
	gateway := api.NewDrive(photoServer.URL, nil, photoServer.Client())
	client := api.NewPhotos(base.String(), notesSourceSession{}, nil)
	openPlan := func(crypto *Client, authority privatePhotos.OriginalPinAuthority) *privatePhotos.SourcePlanStore {
		t.Helper()
		s, err := privatePhotos.NewSourcePlanStore(privatePhotos.SourcePlanConfig{Context: ctx, Scope: scope, Crypto: crypto, Gateway: gateway, Journal: newJournal(), Originals: authority})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		return s
	}
	// Persist an authenticated encrypted plan and simulate losing the entire
	// device before Begin. The next device has an empty upload journal.
	epoch := uuid.NewString()
	info, err := client.StartPrivateSourcePreflight(ctx, scope, epoch)
	if err != nil {
		t.Fatal(err)
	}
	planStore := openPlan(photoDevice.Client, pins)
	ref, err := planStore.PublishPagedSourcePlan(ctx, epoch, client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driveRepository.Upload(ctx, driveSource.Root, "private.bin", "application/octet-stream", bytes.NewReader([]byte("replacement")), 11); err != nil {
		t.Fatal(err)
	}
	fresh := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), photoDevice)
	freshPlan := openPlan(fresh, pins)
	source, err := privatePhotos.NewPlannedHTTPSource(scope, client, freshPlan)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := source.SnapshotEpoch(ctx, scope)
	if err != nil || retained != epoch {
		t.Fatal("accepted epoch lost after device crash", err)
	}
	repository, err := privatePhotos.New(privatePhotos.Config{Context: ctx, Scope: scope, Crypto: fresh, Gateway: gateway, Journal: newJournal(), Source: source})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repository.Close)
	if err := repository.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal("actual SQL/HTTP/composite import", err)
	}
	state, err := source.State(ctx, scope)
	wantRows, wantBlobs, countErr := (privatePhotos.SourcePlanCounts{BackendRows: info.Rows, BackendBlobs: info.Blobs, OriginalRows: 2 * info.Originals, OriginalBlobs: info.Originals}).Totals()
	if err != nil || countErr != nil || state.Epoch != epoch || state.Rows != wantRows || state.Blobs != wantBlobs {
		t.Fatal("composite source lost rows or blobs", state, err)
	}
	verifyPhotosSourceBytes(ctx, t, repository, source, scope, state)
	var body bytes.Buffer
	if err := repository.Original(ctx, asset, &body); err != nil || !bytes.Equal(body.Bytes(), original) {
		t.Fatal("original followed replaced Drive head", err)
	}
	audit, err := repository.AuditBinaries(ctx)
	if err != nil || audit.Objects != 3 {
		t.Fatal("private bytes availability proof", audit, err)
	}
	if err := fresh.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	next := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), photoDevice)
	// No Drive authority is available on the independently recovered device.
	// Its private catalog and original bytes come from Photos ciphertext only.
	withoutDrive := openPlan(next, nil)
	nextSource, err := privatePhotos.NewPlannedHTTPSource(scope, client, withoutDrive)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := nextSource.State(ctx, scope)
	if err != nil || metadata.Epoch != ref.Epoch || metadata.Rows != wantRows {
		t.Fatal("fresh metadata required Drive", err)
	}
	counted := &countedPhotosSource{Source: nextSource}
	recovered, err := privatePhotos.New(privatePhotos.Config{Context: ctx, Scope: scope, Crypto: next, Gateway: gateway, Journal: newJournal(), Source: counted})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(recovered.Close)
	if err := recovered.Initialize(ctx); err != nil {
		t.Fatal("fresh private catalog", err)
	}
	body.Reset()
	if err := recovered.Original(ctx, asset, &body); err != nil || !bytes.Equal(body.Bytes(), original) || counted.reads != 0 {
		t.Fatal("fresh device read legacy source or required Drive", err, counted.reads)
	}
	for _, raw := range photoObjects.data {
		if bytes.Contains(raw, original) || bytes.Contains(raw, []byte("retained.png")) || bytes.Contains(raw, []byte("exact historical thumbnail")) {
			t.Fatal("plaintext reached private blob gateway")
		}
	}
	t.Log("PHOTOS_PG_HTTP_SDK_FRESH_PASS accepted-plan crash before Begin, exact retained SQL and historic variants, replaced Drive original pinned, fresh SSSS Photos recovery without Drive or source reads")
}

type countedPhotosSource struct {
	privatePhotos.Source
	reads int
}

func (s *countedPhotosSource) Page(ctx context.Context, scope storage.Scope, epoch string, cursor storage.LegacySourceCursor) (privatePhotos.SnapshotPage, error) {
	s.reads++
	return s.Source.Page(ctx, scope, epoch, cursor)
}

func (s *countedPhotosSource) Open(ctx context.Context, scope storage.Scope, epoch string, row privatePhotos.SnapshotRow) (io.ReadCloser, error) {
	s.reads++
	return s.Source.Open(ctx, scope, epoch, row)
}

func verifyPhotosSourceBytes(ctx context.Context, t *testing.T, repository *privatePhotos.Repository, source privatePhotos.Source, scope storage.Scope, state privatePhotos.SnapshotState) {
	t.Helper()
	cursor := storage.LegacySourceCursor{}
	var count int64
	for {
		page, err := source.Page(ctx, scope, state.Epoch, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Rows {
			count++
			reader, err := source.Open(ctx, scope, state.Epoch, row)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil || closeErr != nil || int64(len(raw)) != row.Size || digestBytes(raw) != row.SHA256 {
				t.Fatal("source bytes mismatch", err, closeErr)
			}
			if row.Kind == "variant" {
				var got bytes.Buffer
				if err := repository.ReadVariant(ctx, row.ID, &got); err != nil || !bytes.Equal(got.Bytes(), raw) {
					t.Fatal("historical variant mismatch", err)
				}
			} else if row.Kind != "original" {
				got, err := repository.ReadRaw(ctx, row.Kind, row.ID)
				if err != nil || !bytes.Equal(got, raw) {
					t.Fatal("SQL JSON mismatch", row.Kind, err)
				}
				clear(got)
			}
			clear(raw)
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if count != state.Rows {
		t.Fatal("incomplete retained inventory", count, state.Rows)
	}
}
