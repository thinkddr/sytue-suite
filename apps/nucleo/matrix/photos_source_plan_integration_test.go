// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type planCASFixture struct {
	scope               storage.Scope
	epoch, ticket, mode string
	link                storage.Reference
	calls               int
}

type deniedPlanReadback struct {
	*Client
	denied error
}

func (c deniedPlanReadback) DownloadStorageFile(context.Context, *api.Drive, storage.Reference, string, string, io.Writer) (storage.FileRequest, error) {
	return storage.FileRequest{}, c.denied
}

func (f *planCASFixture) AcceptSourcePlan(_ context.Context, scope storage.Scope, epoch, ticket string, link storage.Reference) (privatePhotos.SourcePlanReference, error) {
	f.calls++
	if scope != f.scope || epoch != f.epoch || ticket != f.ticket || !link.Valid() || f.link.Valid() && f.link != link {
		return privatePhotos.SourcePlanReference{}, errors.New("changed preflight or plan link")
	}
	f.link = link
	if f.mode == "reject" {
		return privatePhotos.SourcePlanReference{}, errors.New("synthetic rejected inventory CAS")
	}
	if f.mode == "changed" {
		link.EventID = "$different_accepted_plan"
	}
	return privatePhotos.SourcePlanReference{Epoch: epoch, Link: link}, nil
}

func TestPhotosSourcePlanAgainstSynapsePinsDriveBeforeCASAndRecoversFresh(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PHOTOS_SOURCE_PLAN") != "1" {
		t.Skip("requires isolated disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	upstream, err := url.Parse(os.Getenv("SYTUE_MATRIX_PHOTOS_SOURCE_PLAN_URL"))
	if err != nil || upstream.Host == "" {
		t.Fatal("disposable Synapse URL required")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "photos"}
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
	driveObjects := &fileGatewayFixture{product: "drive", policyScope: driveScope}
	driveServer := httptest.NewServer(driveObjects)
	t.Cleanup(driveServer.Close)
	driveGateway := api.NewDrive(driveServer.URL, nil, driveServer.Client())
	driveSource := syntheticIdentitySource(t, driveScope)
	// Keep this fixture's index JSON inline so the media read counter measures
	// original bodies independently of large JSON metadata covered elsewhere.
	for i, row := range driveSource.Rows {
		if row.Kind == "file" && row.ID == driveSource.File {
			var fields map[string]json.RawMessage
			if json.Unmarshal(row.Payload, &fields) != nil {
				t.Fatal("synthetic file row")
			}
			delete(fields, "large_private_metadata")
			driveSource.Rows[i].Payload, _ = json.Marshal(fields)
		}
	}
	openDrive := func(crypto *Client) *privateDrive.Repository {
		t.Helper()
		r, err := privateDrive.New(privateDrive.Config{Context: ctx, Scope: driveScope, Crypto: crypto, Gateway: driveGateway, Journal: newJournal(), Source: driveSource})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(r.Close)
		if err := r.Initialize(ctx); err != nil {
			t.Fatal(err)
		}
		return r
	}
	driveRepository := openDrive(driveDevice.Client)
	authority, err := privatePhotos.NewDriveOriginalPins(driveRepository)
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.NewString()
	original := bytes.Clone(driveSource.Blobs["file/"+driveSource.File])
	raw := []byte(`{"id":"` + assetID + `","tenant":"acme","owner_sub":"` + scope.Subject + `","drive_file_id":"` + driveSource.File + `","name":"retained.png","media_type":"image/png","bytes":` + strconv.Itoa(len(original)) + `,"width":16,"height":12,"orientation":1,"uploaded_at":"2026-10-30T10:00:00Z","future_integer":900719925474099312345}`)
	snapshot, err := model.NewSnapshot(scope, []model.Row{{Kind: "asset", ID: assetID, JSON: raw}})
	if err != nil {
		t.Fatal(err)
	}
	backend := []storage.StreamSourceRow{{Kind: "asset", ID: assetID, Size: int64(len(raw)), SHA256: digestBytes(raw)}}
	plan, err := privatePhotos.PrepareSourcePlan(ctx, scope, uuid.NewString(), "synthetic_inventory_CAS", snapshot, backend, 0, authority)
	if err != nil {
		t.Fatal(err)
	}
	photoObjects := &fileGatewayFixture{product: "photos"}
	photoServer := httptest.NewServer(photoObjects)
	t.Cleanup(photoServer.Close)
	photoGateway := api.NewDrive(photoServer.URL, nil, photoServer.Client())
	journal := newJournal()
	openStore := func(crypto privatePhotos.SourcePlanCrypto, pins privatePhotos.OriginalPinAuthority, journal *offline.Store) *privatePhotos.SourcePlanStore {
		t.Helper()
		store, err := privatePhotos.NewSourcePlanStore(privatePhotos.SourcePlanConfig{Context: ctx, Scope: scope, Crypto: crypto, Gateway: photoGateway, Journal: journal, Originals: pins})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(store.Close)
		return store
	}
	store := openStore(photoDevice.Client, authority, journal)
	control := &planCASFixture{scope: scope, epoch: plan.Epoch, ticket: plan.CAS, mode: "reject"}
	readDenied := errors.New("synthetic plan readback unavailable")
	blocked := openStore(deniedPlanReadback{Client: photoDevice.Client, denied: readDenied}, authority, journal)
	if _, err := blocked.Publish(ctx, plan, control); !errors.Is(err, readDenied) || control.calls != 0 {
		t.Fatal("unverified plan reached backend CAS", err)
	}
	if _, err := store.Publish(ctx, plan, control); err == nil || !control.link.Valid() {
		t.Fatal("rejected CAS lost retained plan", err)
	}
	if count, err := journal.Count(storage.PendingUploadsNamespace); err != nil || count != 1 {
		t.Fatal("CAS rejection discarded SDK WAL", count, err)
	}
	if count, err := journal.Count("photos-private-stage-v1"); err != nil || count == 0 {
		t.Fatal("CAS rejection discarded encrypted plan input", count, err)
	}
	before := photoObjects.puts
	control.mode = "changed"
	if _, err := store.Publish(ctx, plan, control); err == nil || photoObjects.puts != before {
		t.Fatal("changed accepted link acknowledged or reuploaded plan", err)
	}
	if count, err := journal.Count(storage.PendingUploadsNamespace); err != nil || count != 1 {
		t.Fatal("changed reference retired WAL", count, err)
	}
	// Moving the Drive head after capture must not redirect saved Photos pins.
	changed := []byte("a different replacement original")
	if _, err := driveRepository.Upload(ctx, driveSource.Root, "private.bin", "application/octet-stream", bytes.NewReader(changed), int64(len(changed))); err != nil {
		t.Fatal(err)
	}
	if err := driveDevice.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := photoDevice.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	freshDrive := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), driveDevice)
	freshPhotos := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), photoDevice)
	freshAuthority, err := privatePhotos.NewDriveOriginalPins(openDrive(freshDrive))
	if err != nil {
		t.Fatal(err)
	}
	freshStore := openStore(freshPhotos, freshAuthority, newJournal())
	ref := privatePhotos.SourcePlanReference{Epoch: plan.Epoch, Link: control.link}
	beforeMedia := driveObjects.gets
	recovered, err := freshStore.Read(ctx, ref)
	if err != nil || driveObjects.gets != beforeMedia {
		t.Fatal("fresh plan validation downloaded originals", err)
	}
	if recovered.Counts != plan.Counts || len(recovered.Originals) != 1 || recovered.Originals[0].Descriptor != plan.Originals[0].Descriptor {
		t.Fatal("fresh plan changed composite counts or immutable original")
	}
	var output bytes.Buffer
	if err := freshAuthority.Write(ctx, scope, recovered.Originals[0], &output); err != nil || !bytes.Equal(output.Bytes(), original) {
		t.Fatal("fresh saved pin followed replacement head", err)
	}
	control.mode = "accept"
	accepted, err := store.Publish(ctx, plan, control)
	if err != nil || accepted != ref || photoObjects.puts != before {
		t.Fatal("exact accepted retry lost retained plan", err)
	}
	for _, ns := range []string{"photos-private-v1", "photos-private-stage-v1", storage.PendingUploadsNamespace} {
		if count, err := journal.Count(ns); err != nil || count != 0 {
			t.Fatal("verified accepted plan retained local work", ns, count, err)
		}
	}
	planJSON, _ := json.Marshal(plan)
	for _, raw := range photoObjects.data {
		if bytes.Contains(raw, planJSON) || bytes.Contains(raw, []byte("retained-drive-id")) {
			t.Fatal("server received clear plan")
		}
	}
	t.Log("real SDK: Drive pin before CAS, changed-head retention, rejected/changed-ref CAS keeps WAL, encrypted sidecar, fresh product identities validate without media reads and recover original bytes")
}
