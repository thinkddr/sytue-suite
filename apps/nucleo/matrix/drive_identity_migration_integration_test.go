// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func controlledMigrationSource(ctx context.Context, t *testing.T, home string, httpClient *http.Client, scope storage.Scope) (storageSynapseDevice, *mautrix.Client) {
	t.Helper()
	source := newStorageSynapseDeviceForScope(ctx, t, home, httpClient, scope, storage.LegacyIdentity)
	controller, err := mautrix.NewClient(home, "", "")
	if err != nil {
		t.Fatal(err)
	}
	controller.Client = httpClient
	matrixprooftest.Register(ctx, t, controller, "rooms_controller", rand.Text(), "METADATA_CONTROLLER")
	roomID := matrixprooftest.EncryptedStorageRoom(ctx, t, controller, source.Matrix.UserID)
	if _, err := source.Matrix.JoinRoom(ctx, roomID.String(), nil); err != nil {
		t.Fatal(err)
	}
	if err := source.Client.Close(); err != nil {
		t.Fatal(err)
	}
	source.Room = roomID
	source.Config.Bootstrap.ControllerUserID = controller.UserID.String()
	source.Config.Bootstrap.StorageRoomID = roomID.String()
	source.Config.Bootstrap.StorageRoomIDs = []string{roomID.String()}
	client, err := Open(ctx, source.Config)
	if err != nil {
		t.Fatal("reopen controlled source", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	source.Client = client
	go func() { _ = client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	return source, controller
}

func migrationFixturePowers(frozen bool) map[string]any { return matrixprooftest.StoragePowers(frozen) }

func syntheticIdentitySource(t *testing.T, scope storage.Scope) *syntheticDriveSource {
	t.Helper()
	started := time.Date(2026, 10, 29, 21, 2, 3, 123456789, time.UTC)
	root, file, version, trash, folder := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	source := &syntheticDriveSource{Root: root, File: file, Folder: folder, Trash: trash, Blobs: map[string][]byte{
		"file/" + file:       []byte("private baseline content version two"),
		"version/" + version: []byte("private older content version one"),
		"file/" + trash:      []byte("private independently trashed content"),
	}}
	add := func(kind, key string, fields map[string]any) {
		payload, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		source.Rows = append(source.Rows, storage.LegacySourceRow{Kind: kind, ID: key, Payload: payload})
	}
	add("folder", root, map[string]any{"id": root, "name": "Mi unidad", "is_root": true, "created_at": started, "updated_at": started})
	add("folder", folder, map[string]any{"id": folder, "name": "Private folder", "parent_id": root, "created_at": started, "updated_at": started})
	fields := map[string]any{"id": file, "folder_id": root, "name": "private.bin", "size_bytes": len(source.Blobs["file/"+file]), "version": 2, "content_type": "application/octet-stream", "created_at": started, "updated_at": started, "extension": json.RawMessage(`{"integer":900719925474099512345,"scientific":1.234567890123456789e+99}`), "large_private_metadata": strings.Repeat("private metadata ", 1500)}
	add("file", file, fields)
	add("version", version, map[string]any{"id": version, "file_id": file, "version": 1, "size_bytes": len(source.Blobs["version/"+version]), "content_type": "application/octet-stream", "created_at": started})
	add("file", trash, map[string]any{"id": trash, "folder_id": root, "name": "retained-trash.bin", "size_bytes": len(source.Blobs["file/"+trash]), "version": 1, "content_type": "application/octet-stream", "created_at": started, "updated_at": started, "deleted_at": started})
	slices.SortFunc(source.Rows, func(a, b storage.LegacySourceRow) int {
		if n := strings.Compare(a.Kind, b.Kind); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	source.State = storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: uuid.NewString(), StartedAt: &started, Rows: int64(len(source.Rows)), Blobs: int64(len(source.Blobs))}
	return source
}

// This routing port retains public metadata only. Real backend persistence and
// power-level enforcement have their own PostgreSQL/Synapse integration test.
type driveCutoverRouting struct{ state storage.IdentityCutoverState }

func (r *driveCutoverRouting) MatrixStorageMigration(_ context.Context, scope storage.Scope) (storage.IdentityCutoverState, error) {
	if scope != r.state.Scope {
		return storage.IdentityCutoverState{}, ErrIdentity
	}
	return r.state, nil
}

func (r *driveCutoverRouting) BeginMatrixStorageMigration(ctx context.Context, scope storage.Scope, epoch string) (storage.IdentityCutoverState, error) {
	state, err := r.MatrixStorageMigration(ctx, scope)
	if err != nil || epoch != state.Epoch || state.Phase != "frozen" {
		return storage.IdentityCutoverState{}, ErrIdentity
	}
	return state, nil
}

func (r *driveCutoverRouting) CompleteMatrixStorageMigration(_ context.Context, scope storage.Scope, epoch string, receipt storage.Reference) (storage.IdentityCutoverState, error) {
	if scope != r.state.Scope || epoch != r.state.Epoch || !receipt.Valid() || slices.Contains(r.state.SourceRooms, receipt.RoomID) {
		return storage.IdentityCutoverState{}, ErrIdentity
	}
	r.state.Phase, r.state.Receipt = "isolated", &receipt
	return r.state, nil
}

func TestEncryptedDriveCatalogMigratesAllHistoryToIsolatedIdentity(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_CATALOG_MIGRATION") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	origin := os.Getenv("SYTUE_MATRIX_CATALOG_MIGRATION_URL")
	if origin == "" {
		origin = "http://127.0.0.1:18585"
	}
	upstreamURL, err := url.Parse(origin)
	if err != nil || upstreamURL.Host == "" {
		t.Fatal("disposable migration Synapse URL required", err)
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstreamURL))
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Subject: uuid.NewString(), Tenant: "acme", Product: "drive"}
	old, controller := controlledMigrationSource(ctx, t, proxy.URL, proxy.Client(), scope)
	next := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	files, gateway, _, _ := fileFixture(t)
	gateway.policyScope = scope
	source := syntheticIdentitySource(t, scope)
	secrets := &authtest.Secrets{}
	directory := t.TempDir()
	journal, err := offline.OpenWithSecrets(directory, secrets)
	if err != nil {
		t.Fatal(err)
	}
	open := func(crypto privateDrive.Crypto) *privateDrive.Repository {
		t.Helper()
		repository, err := privateDrive.New(privateDrive.Config{Context: ctx, Scope: scope, Crypto: crypto, Gateway: files.gateway, Source: source, Journal: journal})
		if err != nil {
			t.Fatal(err)
		}
		return repository
	}
	legacy := open(old.Client)
	defer legacy.Close()
	if err = legacy.Initialize(ctx); err != nil {
		t.Fatal("initial private baseline", err)
	}
	if err = legacy.Rename(ctx, api.DriveFile, source.File, "renamed.bin"); err != nil {
		t.Fatal(err)
	}
	changed := []byte("private content edited after the PostgreSQL baseline")
	if _, err = legacy.Upload(ctx, source.Root, "renamed.bin", "application/octet-stream", bytes.NewReader(changed), int64(len(changed))); err != nil {
		t.Fatal("new private version", err)
	}
	if err = legacy.SetFavorite(ctx, api.DriveFile, source.File, true); err != nil {
		t.Fatal(err)
	}
	if err = legacy.Delete(ctx, api.DriveFile, source.File); err != nil {
		t.Fatal(err)
	}
	if err = legacy.Restore(ctx, api.DriveFile, source.File); err != nil {
		t.Fatal(err)
	}
	if err = legacy.Rename(ctx, api.DriveFolder, source.Folder, "Edited after baseline"); err != nil {
		t.Fatal(err)
	}
	expectedFile, err := legacy.ReadRaw(ctx, "file", source.File)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(expectedFile)
	versions, _, err := legacy.Versions(ctx, source.File, "")
	if err != nil || len(versions) < 3 {
		t.Fatal("fixture did not retain original and new versions", len(versions), err)
	}
	pages, opens := source.pages, source.opens
	target := open(next.Client)
	copyFile := func(ctx context.Context, target storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
		return old.Client.MigrateStorageFileContentAt(ctx, next.Client, files.gateway, journal, job, target, ref, object, revision)
	}
	if err = target.MigratePrivateCatalog(ctx, legacy, copyFile); !errors.Is(err, ErrStorageNotFrozen) {
		t.Fatal("writable source accepted", err)
	}
	target.Close()
	if _, err = controller.SendStateEvent(ctx, old.Room, event.StatePowerLevels, "", migrationFixturePowers(true)); err != nil {
		t.Fatal("freeze real Matrix policy", err)
	}
	if _, err = old.Client.FrozenStorageBindings(ctx); err != nil {
		t.Fatal("actual read-only policy not recognized", err)
	}
	blocked := storage.Record{Version: 1, Scope: scope, ObjectID: "must-not-publish", RevisionID: "blocked", Role: "content", Payload: []byte(`{"synthetic":"blocked by Matrix power levels"}`)}
	if _, err = old.Client.PutStorageRecord(ctx, "fenced-write", blocked); err == nil {
		t.Fatal("Synapse accepted a write after source fence")
	}
	faults := &lostDriveImporterACK{Client: next.Client, contents: map[string]storage.Reference{}, catalogs: map[string]storage.Reference{}}
	routing := &driveCutoverRouting{state: storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "frozen", Epoch: uuid.NewString(), SourceRooms: []string{old.Room.String()}}}
	for attempt := 0; attempt < 3; attempt++ {
		target = open(faults)
		cutover, createErr := privateDrive.NewIdentityCutover(privateDrive.IdentityCutoverConfig{Scope: scope, Service: routing, Legacy: legacy, Isolated: target, CopyFile: copyFile, Authorize: func(ctx context.Context) error { return ctx.Err() }})
		if createErr != nil {
			t.Fatal(createErr)
		}
		err = cutover.Prepare(ctx)
		if err == nil {
			break
		}
		if _, rootErr := target.Root(ctx); !errors.Is(rootErr, privateDrive.ErrNotReady) {
			t.Fatal("partial destination exposed after lost ACK", rootErr)
		}
		target.Close()
		if attempt == 2 {
			t.Fatal("catalog migration did not resume", err)
		}
		journal, err = offline.OpenWithSecrets(directory, secrets)
		if err != nil {
			t.Fatal(err)
		}
	}
	defer target.Close()
	if routing.state.Phase != "isolated" || routing.state.Receipt == nil {
		t.Fatal("complete graph did not acknowledge isolated routing")
	}
	if !faults.contentLost || !faults.catalogLost {
		t.Fatal("both accepted ACK failures were not exercised")
	}
	if source.pages != pages || source.opens != opens {
		t.Fatal("encrypted catalog migration read obsolete SQL source", source.pages, source.opens, pages, opens)
	}
	actual, err := target.ReadRaw(ctx, "file", source.File)
	if err != nil || !bytes.Equal(actual, expectedFile) {
		t.Fatal("new identity lost private edits or exact metadata", err)
	}
	clear(actual)
	current, err := target.Download(ctx, source.File)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(current)
	_ = current.Close()
	if err != nil || !bytes.Equal(data, changed) {
		t.Fatal("new identity reverted to SQL baseline", err)
	}
	clear(data)
	migratedVersions, _, err := target.Versions(ctx, source.File, "")
	expectedJSON, _ := json.Marshal(versions)
	actualJSON, _ := json.Marshal(migratedVersions)
	if err != nil || !bytes.Equal(expectedJSON, actualJSON) {
		t.Fatal("version metadata or complete history changed", err)
	}
	for _, version := range migratedVersions {
		if version.IsCurrent {
			continue
		}
		reader, err := target.DownloadVersion(ctx, source.File, version.Version)
		if err != nil {
			t.Fatal("historical content missing", version.Version, err)
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil || len(data) == 0 {
			t.Fatal("historical body not recovered", version.Version, err)
		}
		clear(data)
	}
	favorites, err := target.Favorites(ctx)
	if err != nil || len(favorites) != 1 || favorites[0].ID != source.File {
		t.Fatal("favorite identity lost", err)
	}
	trash, err := target.Trash(ctx)
	if err != nil || len(trash.Files) != 1 || trash.Files[0].ID != source.Trash {
		t.Fatal("independent trash lost", err)
	}
	target.Close()
	target = open(next.Client)
	defer target.Close()
	if err = target.Initialize(ctx); err != nil {
		t.Fatal("new identity cannot reopen its transferred import receipt", err)
	}
	if source.pages != pages || source.opens != opens {
		t.Fatal("reopening reimported SQL")
	}
	pdfProof := exercisePrivatePDFStorage(ctx, t, target)
	target = exerciseConditionalPrivatePDFRecovery(ctx, t, target, open, faults, gateway, &pdfProof)
	defer target.Close()
	if err = next.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	anonymous, err := mautrix.NewClient(proxy.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	anonymous.Client = proxy.Client()
	login, err := anonymous.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: next.Matrix.UserID.String()}, Password: next.Password, DeviceID: "CATALOG_FRESH"})
	if err != nil {
		t.Fatal(err)
	}
	sharedRecovery := &memorySecrets{values: map[string][]byte{}}
	recoveryConfig := &NativeRecoveryConfig{Binding: strings.Repeat("a", 32), Secrets: sharedRecovery, Authorize: func(ctx context.Context) error { return ctx.Err() }}
	next.Client.cfg.NativeRecovery = recoveryConfig
	cfg := next.Config
	cfg.NativeConsumer = "pdf"
	cfg.NativeRecovery = recoveryConfig
	cfg.StoreDir = t.TempDir()
	cfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	fresh := storageSynapseSession(ctx, t, cfg, credentials{NativeConsumer: "pdf", Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: scope.Subject, Tenant: scope.Tenant})
	if fresh.account == next.Client.account || fresh.DeviceID() == next.Client.DeviceID() || bytes.Equal(fresh.master, next.Client.master) {
		t.Fatal("PDF consumer reused Drive device credentials or private database key")
	}
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err = fresh.Recover(ctx, old.Recovery); err == nil {
		t.Fatal("legacy recovery unlocked new catalog")
	}
	if err = fresh.SyncNativeRecovery(ctx); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("unverified PDF consumer bypassed absent authorized OS recovery", err)
	}
	if err = next.Client.SyncNativeRecovery(ctx); err != nil {
		t.Fatal("acknowledged Drive owner could not publish scoped OS recovery", err)
	}
	if err = fresh.SyncNativeRecovery(ctx); err != nil {
		t.Fatal("fresh independent PDF device could not recover from scoped OS material", err)
	}
	journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal("fresh empty private journal", err)
	}
	recovered := open(fresh)
	defer recovered.Close()
	if err = recovered.Initialize(ctx); err != nil {
		t.Fatal("fresh recovered repository", err)
	}
	actual, err = recovered.ReadRaw(ctx, "file", source.File)
	if err != nil || !bytes.Equal(actual, expectedFile) {
		t.Fatal("fresh device lost copied private graph", err)
	}
	clear(actual)
	if source.pages != pages || source.opens != opens {
		t.Fatal("fresh device read SQL originals")
	}
	checkRecoveredPrivatePDFStorage(ctx, t, recovered, pdfProof, gateway)
	updatedPDF, err := privatePDFClient(t, recovered).Aplicar(ctx, pdfProof.survivor, api.PDFFichero{Nombre: pdfProof.survivor.Name, Datos: pdfProof.survivorBytes}, "", api.PDFCambio{Op: "metadatos", Datos: map[string]string{"titulo": "Second independent device edit"}})
	if err != nil {
		t.Fatal("second recovered device could not save its observed version", err)
	}
	stalePDF, err := privatePDFClient(t, target).Aplicar(ctx, pdfProof.survivor, api.PDFFichero{Nombre: pdfProof.survivor.Name, Datos: pdfProof.survivorBytes}, "", api.PDFCambio{Op: "metadatos", Datos: map[string]string{"titulo": "Stale first device edit"}})
	if !errors.Is(err, privateDrive.ErrConflict) || len(stalePDF.Datos) != 0 {
		t.Fatal("first device overwrote second device's recovered edit", err)
	}
	checkPrivatePDFOriginal(ctx, t, privatePDFClient(t, recovered), updatedPDF.Fichero, updatedPDF.Datos)
	pin, err := recovered.PinRetainedOriginal(ctx, "file", source.File)
	if err != nil || pin.Scope != scope {
		t.Fatal("fresh device could not pin an authenticated Drive original", err)
	}
	replacement := []byte("later replacement must never redirect an existing media pin")
	updated, err := recovered.Upload(ctx, source.Root, "renamed.bin", "application/octet-stream", bytes.NewReader(replacement), int64(len(replacement)))
	if err != nil || updated.ID != source.File {
		t.Fatal("private replacement fixture did not update the pinned file", err)
	}
	var pinned bytes.Buffer
	if err = recovered.WritePinnedOriginal(ctx, pin, &pinned); err != nil || !bytes.Equal(pinned.Bytes(), changed) {
		t.Fatal("SDK pin followed the replacement instead of its immutable original", err)
	}
	clear(pinned.Bytes())
	trashPin, err := recovered.PinRetainedOriginal(ctx, "file", source.Trash)
	if err != nil {
		t.Fatal("could not pin retained trash for suite migration", err)
	}
	pinned.Reset()
	if err = recovered.WritePinnedOriginal(ctx, trashPin, &pinned); err != nil || !bytes.Equal(pinned.Bytes(), source.Blobs["file/"+source.Trash]) {
		t.Fatal("retained trash pin lost its authenticated body", err)
	}
	clear(pinned.Bytes())
	oldData, err := legacy.ReadRaw(ctx, "file", source.File)
	if err != nil || !bytes.Equal(oldData, expectedFile) {
		t.Fatal("migration retired original before cleanup", err)
	}
	clear(oldData)
	t.Log("independent native PDF device recovered through validated OS SSSS material; real Synapse: controlled read-only source, complete private Drive graph including post-SQL edits/versions/trash/favorites/large exact JSON, accepted content/index ACK failures and journal reopen, no SQL reimport, independent fresh isolated recovery and retained originals verified")
}
