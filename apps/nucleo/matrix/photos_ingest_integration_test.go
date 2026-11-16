// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"sync/atomic"
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

// ingestBackendSource is the empty private backend catalog: the import on the
// verified device copies zero rows, so every photo in the graph comes from the
// ingest itself.
type ingestBackendSource struct {
	scope  storage.Scope
	state  privatePhotos.SnapshotState
	begins int
}

func (s *ingestBackendSource) State(context.Context, storage.Scope) (privatePhotos.SnapshotState, error) {
	return s.state, nil
}

func (s *ingestBackendSource) Begin(_ context.Context, scope storage.Scope, epoch string) (privatePhotos.SnapshotState, error) {
	s.begins++
	now := time.Now().UTC()
	s.state = privatePhotos.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now}}
	return s.state, nil
}

func (s *ingestBackendSource) Page(_ context.Context, scope storage.Scope, epoch string, _ storage.LegacySourceCursor) (privatePhotos.SnapshotPage, error) {
	return privatePhotos.SnapshotPage{Scope: scope, Epoch: epoch}, nil
}

func (s *ingestBackendSource) Open(context.Context, storage.Scope, string, privatePhotos.SnapshotRow) (io.ReadCloser, error) {
	return nil, errors.New("empty ingest backend has no bodies")
}

// ingestPlanControl accepts exactly one empty plan; the accepted reference then
// routes every later device to the same retained plan.
type ingestPlanControl struct {
	scope storage.Scope
	cas   string
	ref   *privatePhotos.SourcePlanReference
	calls int
}

func (c *ingestPlanControl) PrivateSourcePlanRouting(_ context.Context, scope storage.Scope) (model.SourcePlanRouting, error) {
	if scope != c.scope {
		return model.SourcePlanRouting{}, errors.New("ingest routing scope")
	}
	return model.SourcePlanRouting{Scope: c.scope, Plan: c.ref}, nil
}

func (c *ingestPlanControl) PrivateSourcePreflight(_ context.Context, scope storage.Scope, epoch string) (model.SourcePreflight, error) {
	return model.SourcePreflight{Scope: scope, Epoch: epoch, CAS: c.cas}, nil
}

func (c *ingestPlanControl) AcceptSourcePlan(_ context.Context, scope storage.Scope, epoch, ticket string, link storage.Reference) (privatePhotos.SourcePlanReference, error) {
	c.calls++
	if scope != c.scope || ticket != c.cas || !link.Valid() {
		return privatePhotos.SourcePlanReference{}, errors.New("ingest plan CAS mismatch")
	}
	ref := privatePhotos.SourcePlanReference{Epoch: epoch, Link: link}
	c.ref = &ref
	return ref, nil
}

// ingestFaultClient crashes the ingest deterministically in one of its two
// windows: "publish" cancels right after the media row index is published (the
// mutation is left half-done, its WAL resumes), and "ack" cancels right after
// the retained original link is acknowledged (the media mutation completes and
// the asset mutation never starts).
type ingestFaultClient struct {
	*Client
	mode   string
	armed  atomic.Bool
	cancel func()
}

func (c *ingestFaultClient) fire() {
	if c.armed.CompareAndSwap(true, false) {
		c.cancel()
	}
}

func (c *ingestFaultClient) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	version, err := c.Client.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
	if err == nil && (c.mode == "publish" || c.mode == "asset") {
		var summary struct {
			Entity string `json:"entity"`
			Seed   bool   `json:"seed"`
		}
		if json.Unmarshal(commit.Metadata, &summary) == nil && !summary.Seed && ((c.mode == "publish" && summary.Entity == "media") || (c.mode == "asset" && summary.Entity == "asset")) {
			c.fire()
		}
	}
	return version, err
}

func (c *ingestFaultClient) AcknowledgeStorageFileLink(ctx context.Context, journal *offline.Store, job string, ref storage.Reference) error {
	err := c.Client.AcknowledgeStorageFileLink(ctx, journal, job, ref)
	if err == nil && c.mode == "ack" {
		c.fire()
	}
	return err
}

func ingestPicture(t *testing.T, seed byte, width, height int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.Set(x, y, color.RGBA{R: seed, G: byte(x), B: byte(y), A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestPhotosIngestAgainstSynapseRecoversBothCrashWindows(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PHOTOS_INGEST") != "1" {
		t.Skip("requires isolated disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	upstream, err := url.Parse(os.Getenv("SYTUE_MATRIX_PHOTOS_INGEST_URL"))
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
		journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
		if err != nil {
			t.Fatal(err)
		}
		return journal
	}

	driveObjects := &fileGatewayFixture{product: "drive", policyScope: driveScope}
	driveServer := httptest.NewServer(driveObjects)
	t.Cleanup(driveServer.Close)
	driveGateway := api.NewDrive(driveServer.URL, nil, driveServer.Client())
	driveSource := syntheticIdentitySource(t, driveScope)
	openDrive := func(crypto *Client) *privateDrive.Repository {
		t.Helper()
		repository, err := privateDrive.New(privateDrive.Config{Context: ctx, Scope: driveScope, Crypto: crypto, Gateway: driveGateway, Journal: newJournal(), Source: driveSource})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(repository.Close)
		if err := repository.Initialize(ctx); err != nil {
			t.Fatal(err)
		}
		return repository
	}
	driveRepository := openDrive(driveDevice.Client)

	photoObjects := &fileGatewayFixture{product: "photos", policyScope: scope}
	photoServer := httptest.NewServer(photoObjects)
	t.Cleanup(photoServer.Close)
	photoGateway := api.NewDrive(photoServer.URL, nil, photoServer.Client())
	fault := &ingestFaultClient{Client: photoDevice.Client, mode: "publish"}
	ingestCtx, ingestCancel := context.WithCancel(ctx)
	fault.cancel = ingestCancel
	defer ingestCancel()

	mount := func(crypto privatePhotos.SourcePlanCrypto, backend *ingestBackendSource, control *ingestPlanControl, authority privatePhotos.OriginalPinAuthority, journal *offline.Store) *privatePhotos.Repository {
		t.Helper()
		plans, err := privatePhotos.NewSourcePlanStore(privatePhotos.SourcePlanConfig{Context: ctx, Scope: scope, Crypto: crypto, Gateway: photoGateway, Journal: newJournal(), Originals: authority})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(plans.Close)
		planned, err := privatePhotos.NewPlannedSource(scope, backend, control, plans)
		if err != nil {
			t.Fatal(err)
		}
		repository, err := privatePhotos.New(privatePhotos.Config{Context: ctx, Scope: scope, Crypto: crypto, Gateway: photoGateway, Journal: journal, Source: planned})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(repository.Close)
		return repository
	}
	backend := &ingestBackendSource{scope: scope, state: privatePhotos.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}}
	control := &ingestPlanControl{scope: scope, cas: uuid.NewString()}
	authority, err := privatePhotos.NewDriveOriginalPins(driveRepository)
	if err != nil {
		t.Fatal(err)
	}
	journal := newJournal()
	repository := mount(fault, backend, control, authority, journal)
	if err := repository.Initialize(ctx); !errors.Is(err, privatePhotos.ErrNotReady) {
		t.Fatal("empty catalog opened without an explicit import", err)
	}
	if err := repository.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal("empty import", err)
	}
	if control.calls != 1 || control.ref == nil || backend.begins != 1 {
		t.Fatal("empty import did not publish exactly one accepted plan", control.calls, backend.begins)
	}

	// Photo one crashes mid-mutation: the media row is published but the
	// mutation never acknowledges, so its WAL must resume on retry.
	one := ingestPicture(t, 7, 64, 48)
	uploaded, err := driveRepository.Upload(ctx, driveSource.Root, "media-crash.png", "image/png", bytes.NewReader(one), int64(len(one)))
	if err != nil {
		t.Fatal(err)
	}
	fault.mode = "publish"
	fault.armed.Store(true)
	if _, err := repository.Indexar(ingestCtx, uploaded.ID); !errors.Is(err, context.Canceled) || fault.armed.Load() {
		t.Fatal("mid-publication crash did not cancel the ingest", err)
	}
	if count, err := journal.Count("photos-private-v1"); err != nil || count == 0 {
		t.Fatal("mid-publication crash left no resumable WAL", count, err)
	}
	recovered, err := repository.Indexar(ctx, uploaded.ID)
	if err != nil {
		t.Fatal("retry did not resume the interrupted ingest", err)
	}
	for _, namespace := range []string{"photos-private-v1", "photos-private-stage-v1", storage.PendingUploadsNamespace} {
		if count, err := journal.Count(namespace); err != nil || count != 0 {
			t.Fatal("resumed ingest left journal debt", namespace, count, err)
		}
	}
	if recovered.Name != "media-crash.png" || recovered.DriveFileID != uploaded.ID || recovered.SizeBytes != int64(len(one)) || recovered.Width != 64 || recovered.Height != 48 {
		t.Fatal("ingested photo lost its Drive view", recovered)
	}
	var original bytes.Buffer
	if err := repository.Original(ctx, recovered.ID, &original); err != nil || !bytes.Equal(original.Bytes(), one) {
		t.Fatal("retained original changed across the resume", err)
	}
	recipe := model.RecetaVacia()
	recipe.Pasos = append(recipe.Pasos, model.Paso{Tipo: model.PasoGiro, Params: map[string]float64{"grados": 90}})
	fingerprint, err := repository.SaveRecipe(ctx, recovered.ID, recipe, "")
	if err != nil {
		t.Fatal("recipe over an ingested original did not save", err)
	}
	variant, err := repository.VariantDe(ctx, recovered.ID, "grande", fingerprint)
	if err != nil {
		t.Fatal("edited variant of an ingested photo did not render", err)
	}
	derivation, err := io.ReadAll(variant)
	_ = variant.Close()
	if err != nil || len(derivation) == 0 || bytes.Contains(derivation, one) {
		t.Fatal("edited variant of an ingested photo is not a fresh rendering", err)
	}

	// Photo two crashes between the two mutations: the media row is complete
	// and dormant, so nothing breaks and the timeline keeps ignoring it.
	two := ingestPicture(t, 11, 32, 24)
	second, err := driveRepository.Upload(ctx, driveSource.Root, "between-phases.png", "image/png", bytes.NewReader(two), int64(len(two)))
	if err != nil {
		t.Fatal(err)
	}
	ingestCtx, ingestCancel = context.WithCancel(ctx)
	defer ingestCancel()
	fault.cancel = ingestCancel
	fault.mode = "ack"
	fault.armed.Store(true)
	if _, err := repository.Indexar(ingestCtx, second.ID); !errors.Is(err, context.Canceled) || fault.armed.Load() {
		t.Fatal("between-phases crash did not cancel the ingest", err)
	}
	for _, namespace := range []string{"photos-private-v1", "photos-private-stage-v1", storage.PendingUploadsNamespace} {
		if count, err := journal.Count(namespace); err != nil || count != 0 {
			t.Fatal("completed media phase left journal debt", namespace, count, err)
		}
	}
	if _, err := repository.PhotoForDriveFile(ctx, second.ID); !errors.Is(err, privatePhotos.ErrNotFound) {
		t.Fatal("dormant ingest leaked into the timeline before its asset row", err)
	}
	photos, _, err := repository.Timeline(ctx, "", 10, false, "")
	if err != nil || len(photos) != 1 || photos[0].ID != recovered.ID {
		t.Fatal("dormant ingest changed the visible timeline", len(photos), err)
	}

	// Photo three loses its local acknowledgement after the visible asset
	// commits. Seeing that asset on retry must still retire its accepted WAL.
	three := ingestPicture(t, 23, 40, 20)
	third, err := driveRepository.Upload(ctx, driveSource.Root, "asset-crash.png", "image/png", bytes.NewReader(three), int64(len(three)))
	if err != nil {
		t.Fatal(err)
	}
	ingestCtx, ingestCancel = context.WithCancel(ctx)
	defer ingestCancel()
	fault.cancel = ingestCancel
	fault.mode = "asset"
	fault.armed.Store(true)
	if _, err := repository.Indexar(ingestCtx, third.ID); !errors.Is(err, context.Canceled) || fault.armed.Load() {
		t.Fatal("visible-asset crash did not cancel the ingest after publication", err)
	}
	if count, err := journal.Count("photos-private-v1"); err != nil || count == 0 {
		t.Fatal("visible-asset crash left no resumable WAL", count, err)
	}
	if _, err := repository.Indexar(ctx, third.ID); err != nil {
		t.Fatal("visible-asset retry failed", err)
	}
	for _, namespace := range []string{"photos-private-v1", "photos-private-stage-v1", storage.PendingUploadsNamespace} {
		if count, err := journal.Count(namespace); err != nil || count != 0 {
			t.Fatal("visible-asset retry left journal debt", namespace, count, err)
		}
	}

	// A fresh device recovers the catalog with the dormant row in it: the
	// graph still validates, and its own retry completes the ingest.
	if err := driveDevice.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := photoDevice.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	freshDrive := openDrive(freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), driveDevice))
	freshAuthority, err := privatePhotos.NewDriveOriginalPins(freshDrive)
	if err != nil {
		t.Fatal(err)
	}
	freshBackend := &ingestBackendSource{scope: scope, state: backend.state}
	freshControl := &ingestPlanControl{scope: scope, cas: control.cas, ref: control.ref}
	freshRepository := mount(freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), photoDevice), freshBackend, freshControl, freshAuthority, newJournal())
	if err := freshRepository.Initialize(ctx); err != nil {
		t.Fatal("dormant ingest broke the fresh device readiness", err)
	}
	if _, err := freshRepository.PhotoForDriveFile(ctx, second.ID); !errors.Is(err, privatePhotos.ErrNotFound) {
		t.Fatal("fresh device saw the dormant ingest before its completion", err)
	}
	completed, err := freshRepository.Indexar(ctx, second.ID)
	if err != nil {
		t.Fatal("fresh device did not complete the dormant ingest", err)
	}
	var retained bytes.Buffer
	if err := freshRepository.Original(ctx, completed.ID, &retained); err != nil || !bytes.Equal(retained.Bytes(), two) {
		t.Fatal("fresh device did not retain the dormant original", err)
	}
	if completed.Name != "between-phases.png" || completed.Width != 32 || completed.Height != 24 || completed.DriveFileID != second.ID {
		t.Fatal("fresh device completed the ingest with foreign metadata", completed)
	}
	// The native gallery's direct route stages a non-seekable OS input and
	// commits its original plus all three rendered variants in one asset row.
	if err := repository.RefreshPrivateData(ctx); err != nil {
		t.Fatal("refresh after the other device completed the dormant ingest", err)
	}
	direct := ingestPicture(t, 31, 48, 30)
	job := uuid.NewString()
	ingestCtx, ingestCancel = context.WithCancel(ctx)
	defer ingestCancel()
	fault.cancel = ingestCancel
	fault.mode = "asset"
	fault.armed.Store(true)
	facade, err := api.NewPhotos("http://unreachable-legacy.invalid", nil, nil).WithPrivateData(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.ImportarFoto(ingestCtx, job, "direct-private.png", "image/png", bytes.NewBuffer(direct), int64(len(direct))); !errors.Is(err, context.Canceled) || fault.armed.Load() {
		t.Fatal("direct import did not stop after its exact asset commit", err, "context", ingestCtx.Err(), "fault armed", fault.armed.Load())
	}
	if count, err := journal.Count("photos-private-v1"); err != nil || count == 0 {
		t.Fatal("direct import lost its accepted intent", count, err)
	}
	completedDirect, err := facade.ReanudarImportaciones(ctx)
	if err != nil || len(completedDirect) != 1 || completedDirect[0].ID != job || completedDirect[0].DriveFileID != "" {
		t.Fatal("activation did not recover the direct OS import", len(completedDirect), err)
	}
	for _, namespace := range []string{"photos-private-v1", "photos-private-stage-v1", storage.PendingUploadsNamespace} {
		if count, err := journal.Count(namespace); err != nil || count != 0 {
			t.Fatal("direct import recovery left journal debt", namespace, count, err)
		}
	}
	for _, variant := range model.Variantes() {
		body, err := repository.VariantDe(ctx, job, string(variant), "")
		if err != nil {
			t.Fatal("direct import did not retain a variant", variant, err)
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || len(data) == 0 {
			t.Fatal("direct import retained an empty variant", variant, err)
		}
	}
	if err := photoDevice.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	isolated := mount(freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), photoDevice), freshBackend, freshControl, freshAuthority, newJournal())
	if err := isolated.Initialize(ctx); err != nil {
		t.Fatal("fresh empty-journal device rejected direct import", err)
	}
	var exact bytes.Buffer
	if err := isolated.Original(ctx, job, &exact); err != nil || !bytes.Equal(exact.Bytes(), direct) {
		t.Fatal("fresh device did not recover the exact direct original", err)
	}
	for _, variant := range model.Variantes() {
		body, err := isolated.VariantDe(ctx, job, string(variant), "")
		if err != nil {
			t.Fatal("fresh device did not recover a direct variant", variant, err)
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || len(data) == 0 {
			t.Fatal("fresh device recovered an empty variant", variant, err)
		}
	}

	for _, gateway := range []*fileGatewayFixture{driveObjects, photoObjects} {
		gateway.mu.Lock()
		for _, encrypted := range gateway.data {
			if bytes.Contains(encrypted, one) || bytes.Contains(encrypted, two) || bytes.Contains(encrypted, three) || bytes.Contains(encrypted, direct) || bytes.Contains(encrypted, []byte{137, 80, 78, 71, 13, 10, 26, 10}) || bytes.Contains(encrypted, []byte("media-crash.png")) || bytes.Contains(encrypted, []byte("between-phases.png")) {
				gateway.mu.Unlock()
				t.Fatal("provider retained readable image bytes or metadata")
			}
		}
		gateway.mu.Unlock()
	}

	t.Log("real SDK/Synapse: ingest resumes a mid-publication WAL, a between-phases crash stays dormant without journal debt, the fresh device validates the graph and completes it, recipes render over retained Drive originals")
}
