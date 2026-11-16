// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package photos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/google/uuid"
	"maunium.net/go/mautrix"

	"git.sytue.eu/sytue/one/apps/fyne/internal/nativestoragetest"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
)

type albumLostACK struct {
	*mx.Client
	drop              atomic.Bool
	accepted          storage.Reference
	loseFileACK       atomic.Bool
	fileACKLost       atomic.Bool
	puts              atomic.Int32
	maxPage           atomic.Int64
	originalDownloads atomic.Int32
	pageUploads       atomic.Int32
}

func (p *albumLostACK) DownloadStorageFile(ctx context.Context, gateway *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	request, err := p.Client.DownloadStorageFile(ctx, gateway, ref, object, revision, out)
	var header struct {
		Entity string `json:"entity"`
		Row    struct {
			Kind string `json:"kind"`
		} `json:"row"`
	}
	if json.Unmarshal(request.Metadata, &header) == nil && (header.Entity == "original" || header.Row.Kind == "original") {
		p.originalDownloads.Add(1)
	}
	return request, err
}

func (p *albumLostACK) UploadStorageFileAt(ctx context.Context, gateway *api.Drive, journal *offline.Store, job string, target storage.PublicationTarget, request storage.FileRequest, reader io.ReaderAt) (storage.Reference, error) {
	var header struct {
		Row struct {
			Kind string `json:"kind"`
		} `json:"row"`
	}
	if json.Unmarshal(request.Metadata, &header) == nil && header.Row.Kind == "album-page" {
		p.pageUploads.Add(1)
		for old := p.maxPage.Load(); request.SizeBytes > old && !p.maxPage.CompareAndSwap(old, request.SizeBytes); old = p.maxPage.Load() {
		}
	}
	return p.Client.UploadStorageFileAt(ctx, gateway, journal, job, target, request, reader)
}

func (p *albumLostACK) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	p.puts.Add(1)
	version, err := p.Client.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
	var header struct {
		Entity string `json:"entity"`
	}
	if err == nil && json.Unmarshal(commit.Metadata, &header) == nil && header.Entity == "album" && p.drop.CompareAndSwap(true, false) {
		p.accepted = version.Reference
		return storage.ObjectVersion{}, errors.New("lost accepted album index ACK")
	}
	return version, err
}

func (p *albumLostACK) AcknowledgeStorageFileLink(ctx context.Context, journal *offline.Store, job string, ref storage.Reference) error {
	err := p.Client.AcknowledgeStorageFileLink(ctx, journal, job, ref)
	if err == nil && p.loseFileACK.CompareAndSwap(true, false) {
		p.fileACKLost.Store(true)
		return errors.New("lost accepted album content ACK")
	}
	return err
}

type privateAlbumSDKFixture struct {
	configuration nativestoragetest.Config
	device        *nativestoragetest.Device
	source        *editorSnapshotSource
	original      []byte
	ids           []string
	albumID       string
	blobs         *storagetest.CipherGateway
	newJournal    func() *offline.Store
	open          func(privatePhotos.Crypto, *offline.Store) *privatePhotos.Repository
}

func newPrivateAlbumSDKFixture(t *testing.T, ctx context.Context, assetCount, memberCount int) privateAlbumSDKFixture {
	t.Helper()
	origin, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_NATIVE_PHOTOS_ALBUMS_URL"))
	if err != nil {
		t.Fatal(err)
	}
	const password = "artificial-disposable-photos-albums-password"
	register := func(name string) *mautrix.Client {
		client, err := mautrix.NewClient(origin.String(), "", "")
		if err != nil {
			t.Fatal(err)
		}
		matrixprooftest.Register(ctx, t, client, name, password, "REGISTRATION")
		return client
	}
	scope := storage.Scope{Tenant: "photos-native-albums", Subject: uuid.NewString(), Product: "photos"}
	controller := register("rooms_controller")
	user := register(strings.TrimSuffix(strings.TrimPrefix(storage.IsolatedMatrixUserPrefix(scope), "@"), ":"))
	room := matrixprooftest.EncryptedStorageRoom(ctx, t, controller, user.UserID)
	configuration := nativestoragetest.Config{Scope: scope, Origin: origin.String(), User: user, Controller: controller, Room: room, Password: password}
	device, err := nativestoragetest.Open(ctx, t, configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := device.Client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	digest := func(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
	source := &editorSnapshotSource{state: privatePhotos.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, bodies: map[string][]byte{}}
	appendRow := func(kind, id string, raw []byte) {
		source.rows = append(source.rows, privatePhotos.SnapshotRow{Kind: kind, ID: id, Size: int64(len(raw)), SHA256: digest(raw)})
		source.bodies[kind+"/"+id] = raw
	}
	original := pngDe(bandas(24, 12))
	ids := make([]string, assetCount)
	for i := range ids {
		ids[i] = uuid.NewString()
	}
	first, albumID := ids[0], uuid.NewString()
	for _, id := range ids {
		asset, _ := json.Marshal(model.AssetRow{ID: id, Tenant: scope.Tenant, Owner: scope.Subject, DriveID: "retained-" + id, Name: "private.png", MediaType: "image/png", Bytes: int64(len(original)), Width: 24, Height: 12, Orientation: 1, UploadedAt: time.Now().UTC()})
		media, _ := json.Marshal(model.MediaRow{ID: id, Tenant: scope.Tenant, Owner: scope.Subject, AssetID: id, Role: "original", Size: int64(len(original)), SHA256: digest(original), ContentType: "image/png"})
		appendRow("asset", id, asset)
		appendRow("media", id, media)
		appendRow("original", id, original)
	}
	albumRaw, _ := json.Marshal(model.AlbumRow{ID: albumID, Tenant: scope.Tenant, Owner: scope.Subject, Title: "Original album", CoverAssetID: &first, CreatedAt: time.Now().UTC()})
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(albumRaw, &fields)
	fields["future_integer"] = json.RawMessage("900719925474099312345")
	albumRaw, _ = json.Marshal(fields)
	appendRow("album", albumID, albumRaw)
	for i, id := range ids[:memberCount] {
		memberID, _ := model.MembershipID(albumID, id)
		member, _ := json.Marshal(model.MembershipRow{Tenant: scope.Tenant, AlbumID: albumID, AssetID: id, Position: i, AddedAt: time.Now().UTC()})
		var extra map[string]json.RawMessage
		_ = json.Unmarshal(member, &extra)
		extra["future_integer"] = json.RawMessage("900719925474099312345")
		member, _ = json.Marshal(extra)
		appendRow("album_asset", memberID, member)
	}
	slices.SortFunc(source.rows, func(a, b privatePhotos.SnapshotRow) int {
		if order := strings.Compare(a.Kind, b.Kind); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	blobs := &storagetest.CipherGateway{Product: "photos"}
	server := httptest.NewServer(blobs)
	t.Cleanup(server.Close)
	gateway := api.NewDrive(server.URL, nil, server.Client())
	newJournal := func() *offline.Store {
		journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
		if err != nil {
			t.Fatal(err)
		}
		return journal
	}
	open := func(crypto privatePhotos.Crypto, journal *offline.Store) *privatePhotos.Repository {
		repository, err := privatePhotos.New(privatePhotos.Config{Context: ctx, Scope: scope, Crypto: crypto, Gateway: gateway, Journal: journal, Source: source})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(repository.Close)
		return repository
	}

	return privateAlbumSDKFixture{configuration: configuration, device: device, source: source, original: original, ids: ids, albumID: albumID, blobs: blobs, newJournal: newJournal, open: open}
}

func TestPrivateAlbumsAgainstSynapsePreserveRelationsAndResumeMetadata(t *testing.T) {
	if os.Getenv("SYTUE_NATIVE_PHOTOS_ALBUMS") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	fixture := newPrivateAlbumSDKFixture(t, ctx, 2, 2)
	configuration, device, source, original := fixture.configuration, fixture.device, fixture.source, fixture.original
	first, second, albumID := fixture.ids[0], fixture.ids[1], fixture.albumID
	newJournal, open := fixture.newJournal, fixture.open
	var err error
	journal := newJournal()
	fault := &albumLostACK{Client: device.Client}
	repository := open(fault, journal)
	if err := repository.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	service := &servicioFalso{}
	application, _ := appDePrueba(t, service)
	application.cli, err = application.cli.WithPrivateData(repository)
	if err != nil {
		t.Fatal(err)
	}
	application.Arrancar()
	application.Ir(RutaAlbumes)
	if len(application.alb.lista) != 1 || application.alb.lista[0].Count != 2 {
		t.Fatal("native albums were not private", application.alb.error)
	}
	application.campoAlbum.PonerTexto("Created on device")
	page := PantallaAlbumes(application.v, application.datosAlbumes())
	form := page.Objects[1].(*fyne.Container).Objects[1].(*fyne.Container)
	test.Tap(form.Objects[1].(*ui.Boton))
	if len(application.alb.lista) != 2 || application.alb.lista[0].Title != "Created on device" {
		t.Fatal("real create control did not persist", application.alb.error)
	}
	application.AbrirAlbum(albumID)
	if len(application.uno.fotos) != 2 || application.uno.fotos[0].ID != first {
		t.Fatal("native album lost retained order", application.uno.error)
	}
	if err := application.cli.Trash(ctx, first); err != nil {
		t.Fatal(err)
	}
	application.AbrirAlbum(albumID)
	if application.uno.album.Count != 1 || application.uno.album.CoverAssetID != second || len(application.uno.fotos) != 1 {
		t.Fatal("native album leaked trashed cover")
	}
	if err := application.cli.Restaurar(ctx, first); err != nil {
		t.Fatal(err)
	}
	fault.drop.Store(true)
	if err := application.cli.RenameAlbum(ctx, albumID, "Accepted private title"); err == nil || !fault.accepted.Valid() {
		t.Fatal("missing lost accepted index ACK")
	}
	application.Cerrar()
	repository.Close()
	if err := device.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := device.Client.Close(); err != nil {
		t.Fatal(err)
	}
	configuration.Recovery = device.Recovery
	peer, err := nativestoragetest.Open(ctx, t, configuration)
	if err != nil {
		t.Fatal(err)
	}
	reader := open(peer.Client, newJournal())
	if err := reader.Initialize(ctx); err != nil {
		t.Fatal("fresh SDK could not open accepted album", err)
	}
	visible, err := reader.Album(ctx, albumID)
	if err != nil || visible.Title != "Accepted private title" || visible.Count != 2 || visible.CoverAssetID != first {
		t.Fatal("fresh device saw incomplete album", visible, err)
	}
	producer, err := mx.Open(ctx, device.Config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = producer.Close() })
	go func() { _ = producer.Sync(ctx, func(mx.DecryptedEvent, error) {}) }()
	if err := producer.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	reopened := open(producer, journal)
	if err := reopened.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := reopened.RenameAlbum(ctx, albumID, "Accepted private title"); err != nil {
		t.Fatal("exact metadata retry failed", err)
	}
	if pending, err := reopened.Pending(ctx); err != nil || pending.Count != 0 {
		t.Fatal("metadata retry retained pending work", pending.Count, err)
	}
	retained, err := reopened.ReadRaw(ctx, "album", albumID)
	if err != nil || !bytes.Contains(retained, []byte("900719925474099312345")) {
		t.Fatal("rename changed unknown JSON", err)
	}
	clear(retained)
	if err := reopened.DeleteAlbum(ctx, albumID); err != nil {
		t.Fatal(err)
	}
	if err := reader.RefreshPrivateData(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Album(ctx, albumID); !errors.Is(err, privatePhotos.ErrNotFound) {
		t.Fatal("deleted album still visible", err)
	}
	if rows, err := reader.Albums(ctx); err != nil || len(rows) != 1 {
		t.Fatal("album deletion changed another album", err)
	}
	for _, id := range []string{first, second} {
		if _, err := reader.Photo(ctx, id); err != nil {
			t.Fatal("album deletion removed a photo", err)
		}
		var body bytes.Buffer
		if err := reader.Original(ctx, id, &body); err != nil || !bytes.Equal(body.Bytes(), original) {
			t.Fatal("album deletion changed original", err)
		}
		memberID, _ := model.MembershipID(albumID, id)
		bodyRaw, err := reader.ReadRaw(ctx, "album_asset", memberID)
		if err != nil || !bytes.Equal(bodyRaw, source.bodies["album_asset/"+memberID]) {
			t.Fatal("album mutation rewrote retained relation", err)
		}
		clear(bodyRaw)
	}
	if len(service.vistas()) != 0 {
		t.Fatal("private native album used legacy HTTP", service.vistas())
	}
}

// The large unknown values belong to historical relationship JSON, not photo
// bytes. No single row exceeds the old row bound; the complete album exceeds
// it. The album index contains one root and its direct pages retain every byte.
func TestPrivateAlbumMembershipAgainstSynapsePagedCheckpointAndRecovery(t *testing.T) {
	if os.Getenv("SYTUE_NATIVE_PHOTOS_ALBUMS") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Minute)
	defer cancel()
	fixture := newPrivateAlbumSDKFixture(t, ctx, 3, 2)
	for _, id := range fixture.ids[:2] {
		memberID, _ := model.MembershipID(fixture.albumID, id)
		key := "album_asset/" + memberID
		var fields map[string]json.RawMessage
		if json.Unmarshal(fixture.source.bodies[key], &fields) != nil {
			t.Fatal("fixture relationship")
		}
		fields["future_payload"], _ = json.Marshal(strings.Repeat("labelled-private-relationship-", (17<<20)/len("labelled-private-relationship-")+1))
		body, _ := json.Marshal(fields)
		fixture.source.bodies[key] = body
		sum := sha256.Sum256(body)
		for i := range fixture.source.rows {
			if fixture.source.rows[i].Kind == "album_asset" && fixture.source.rows[i].ID == memberID {
				fixture.source.rows[i].Size = int64(len(body))
				fixture.source.rows[i].SHA256 = hex.EncodeToString(sum[:])
			}
		}
	}
	journal := fixture.newJournal()
	fault := &albumLostACK{Client: fixture.device.Client}
	repository := fixture.open(fault, journal)
	importStarted := time.Now()
	if err := repository.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("legacy fixture imported in %s", time.Since(importStarted))
	fault.originalDownloads.Store(0)
	service := &servicioFalso{}
	mount := func(repo *privatePhotos.Repository) *App {
		application, _ := appDePrueba(t, service)
		var err error
		application.cli, err = application.cli.WithPrivateData(repo)
		if err != nil {
			t.Fatal(err)
		}
		application.Arrancar()
		return application
	}
	application := mount(repository)
	facade := application.cli
	extra := fixture.ids[2]
	before, _ := repository.ReadRaw(ctx, "album", fixture.albumID)
	beforePuts := fixture.blobs.Snapshot().Puts
	if _, err := facade.AddToAlbum(ctx, fixture.albumID, []string{extra, uuid.NewString()}); !errors.Is(err, privatePhotos.ErrNotFound) {
		t.Fatal("invalid batch was accepted", err)
	}
	after, _ := repository.ReadRaw(ctx, "album", fixture.albumID)
	if !bytes.Equal(before, after) || fixture.blobs.Snapshot().Puts != beforePuts {
		t.Fatal("failed batch exposed partial membership")
	}
	clear(before)
	clear(after)
	if pending, err := repository.Pending(ctx); err != nil || pending.Count != 0 {
		t.Fatal("failed batch retained work", err)
	}
	fault.loseFileACK.Store(true)
	photo, err := facade.Photo(ctx, extra)
	if err != nil {
		t.Fatal(err)
	}
	addStarted := time.Now()
	application.anyadirFotoA(photo, api.Album{ID: fixture.albumID, Title: "Original album"})
	t.Logf("private native Add reached accepted content boundary in %s", time.Since(addStarted))
	if !fault.fileACKLost.Load() || fault.maxPage.Load() <= 16<<20 || fault.originalDownloads.Load() != 0 || fault.pageUploads.Load() != 3 {
		t.Fatal("checkpoint did not retain the large unknown relationship fields", fault.maxPage.Load())
	}
	if pending, err := repository.Pending(ctx); err != nil || pending.Count == 0 {
		t.Fatal("lost content ACK discarded pending checkpoint", err)
	}
	providerPuts := fixture.blobs.Snapshot().Puts
	application.Cerrar()
	repository.Close()
	if err := fixture.device.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.device.Client.Close(); err != nil {
		t.Fatal(err)
	}
	configuration := fixture.configuration
	configuration.Recovery = fixture.device.Recovery
	peerStarted := time.Now()
	peer, err := nativestoragetest.Open(ctx, t, configuration)
	if err != nil {
		t.Fatal(err)
	}
	reader := fixture.open(peer.Client, fixture.newJournal())
	if err := reader.Initialize(ctx); err != nil {
		t.Fatal("fresh SDK could not validate paged checkpoint", err)
	}
	t.Logf("fresh SDK checkpoint initialized in %s", time.Since(peerStarted))
	members, err := reader.AlbumPhotos(ctx, fixture.albumID)
	if err != nil || len(members) != 3 || members[0].ID != fixture.ids[0] || members[1].ID != fixture.ids[1] || members[2].ID != extra {
		t.Fatal("fresh SDK did not reconstruct every checkpoint page in order", len(members), err)
	}
	producer, err := mx.Open(ctx, fixture.device.Config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = producer.Close() })
	go func() { _ = producer.Sync(ctx, func(mx.DecryptedEvent, error) {}) }()
	if err := producer.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	retry := &albumLostACK{Client: producer}
	producerStarted := time.Now()
	reopened := fixture.open(retry, journal)
	if err := reopened.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("producer checkpoint initialized in %s", time.Since(producerStarted))
	if added, err := reopened.AddToAlbum(ctx, fixture.albumID, []string{extra}); err != nil || added != 1 {
		t.Fatal("retry lost its exact original batch result", added, err)
	}
	if retry.puts.Load() != 0 || fixture.blobs.Snapshot().Puts != providerPuts {
		t.Fatal("checkpoint retry republished accepted content", retry.puts.Load())
	}
	if pending, err := reopened.Pending(ctx); err != nil || pending.Count != 0 {
		t.Fatal("checkpoint retry retained pending work", pending.Count, err)
	}
	application = mount(reopened)
	application.AbrirAlbum(fixture.albumID)
	if application.uno.fallo != nil || len(application.uno.fotos) != 3 {
		t.Fatal("native album did not open the authenticated checkpoint projection", application.uno.fallo, len(application.uno.fotos))
	}
	application.quitarDelAlbum(application.uno.fotos[0])
	if application.uno.error != "" {
		t.Fatal("native remove failed", application.uno.error)
	}
	if err := reader.RefreshPrivateData(ctx); err != nil {
		t.Fatal(err)
	}
	visible, err := reader.Album(ctx, fixture.albumID)
	if err != nil || visible.Count != 2 || visible.CoverAssetID != fixture.ids[1] {
		t.Fatal("removing explicit cover did not select remaining first photo", visible, err)
	}
	if err := reopened.Trash(ctx, fixture.ids[1]); err != nil {
		t.Fatal(err)
	}
	if err := reader.RefreshPrivateData(ctx); err != nil {
		t.Fatal(err)
	}
	visible, err = reader.Album(ctx, fixture.albumID)
	if err != nil || visible.Count != 1 || visible.CoverAssetID != fixture.ids[2] {
		t.Fatal("paged membership leaked a trashed photo", visible, err)
	}
	if err := reopened.Restaurar(ctx, fixture.ids[1]); err != nil {
		t.Fatal(err)
	}
	if err := reader.RefreshPrivateData(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range fixture.ids[:2] {
		memberID, _ := model.MembershipID(fixture.albumID, id)
		raw, err := reader.ReadRaw(ctx, "album_asset", memberID)
		if err != nil || !bytes.Equal(raw, fixture.source.bodies["album_asset/"+memberID]) {
			t.Fatal("checkpoint changed unknown historical relationship JSON", err)
		}
		clear(raw)
		var original bytes.Buffer
		if err := reader.Original(ctx, id, &original); err != nil || !bytes.Equal(original.Bytes(), fixture.original) {
			t.Fatal("membership changed original bytes", err)
		}
	}
	application.Cerrar()
	if len(service.vistas()) != 0 {
		t.Fatal("private native membership reached legacy HTTP", service.vistas())
	}
	// Both devices observe the same base, then publish different changes after a
	// common barrier. Neither may silently replace the other's accepted branch.
	if err := reopened.RefreshPrivateData(ctx); err != nil {
		t.Fatal(err)
	}
	arrived, release := make(chan struct{}, 2), make(chan struct{})
	left := fixture.open(&albumIndexBarrier{Client: producer, arrived: arrived, release: release}, fixture.newJournal())
	right := fixture.open(&albumIndexBarrier{Client: peer.Client, arrived: arrived, release: release}, fixture.newJournal())
	if err := left.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := right.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() { _, err := left.AddToAlbum(ctx, fixture.albumID, []string{fixture.ids[0]}); results <- err }()
	go func() { results <- right.RemoveFromAlbum(ctx, fixture.albumID, fixture.ids[1]) }()
	for range 2 {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatal("concurrent album writes did not reach publication barrier")
		}
	}
	close(release)
	for range 2 {
		if err := <-results; err != nil && !errors.Is(err, privatePhotos.ErrConflict) {
			t.Fatal("unexpected concurrent checkpoint result", err)
		}
	}
	left.Close()
	right.Close()
	reopened.Close()
	reader.Close()
	if err := producer.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := peer.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := producer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := peer.Client.Close(); err != nil {
		t.Fatal(err)
	}
	fresh, err := nativestoragetest.Open(ctx, t, configuration)
	if err != nil {
		t.Fatal(err)
	}
	conflicted := fixture.open(fresh.Client, fixture.newJournal())
	if err := conflicted.Initialize(ctx); !errors.Is(err, privatePhotos.ErrConflict) {
		t.Fatal("fresh SDK silently selected a concurrent album branch", err)
	}
}

type albumIndexBarrier struct {
	*mx.Client
	arrived chan<- struct{}
	release <-chan struct{}
}

func (p *albumIndexBarrier) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	var header struct {
		Entity string `json:"entity"`
	}
	if json.Unmarshal(commit.Metadata, &header) == nil && header.Entity == "album" {
		select {
		case p.arrived <- struct{}{}:
		case <-ctx.Done():
			return storage.ObjectVersion{}, ctx.Err()
		}
		select {
		case <-p.release:
		case <-ctx.Done():
			return storage.ObjectVersion{}, ctx.Err()
		}
	}
	return p.Client.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
}
