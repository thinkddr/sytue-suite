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
	"image"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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

// Only the Photos source rows are synthetic. Registration, encrypted rooms,
// SSSS, journal, SDK files, recipe publication, preview and UI remain real.
type editorSnapshotSource struct {
	mu     sync.Mutex
	state  privatePhotos.SnapshotState
	rows   []privatePhotos.SnapshotRow
	bodies map[string][]byte
	opens  int
}

func (s *editorSnapshotSource) State(ctx context.Context, scope storage.Scope) (privatePhotos.SnapshotState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scope != s.state.Scope {
		return privatePhotos.SnapshotState{}, privatePhotos.ErrSource
	}
	return s.state, ctx.Err()
}

func (s *editorSnapshotSource) Begin(ctx context.Context, scope storage.Scope, epoch string) (privatePhotos.SnapshotState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scope != s.state.Scope {
		return privatePhotos.SnapshotState{}, privatePhotos.ErrSource
	}
	if s.state.Mode == "legacy" {
		now := time.Now().UTC()
		blobs := int64(0)
		for _, row := range s.rows {
			if row.Kind == "original" || row.Kind == "variant" {
				blobs++
			}
		}
		s.state.Mode, s.state.Epoch, s.state.StartedAt, s.state.Rows, s.state.Blobs = "frozen", epoch, &now, int64(len(s.rows)), blobs
	}
	return s.state, ctx.Err()
}

func (s *editorSnapshotSource) Page(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (privatePhotos.SnapshotPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := privatePhotos.SnapshotPage{Scope: scope, Epoch: epoch, Rows: []privatePhotos.SnapshotRow{}}
	if scope != s.state.Scope || epoch != s.state.Epoch {
		return page, privatePhotos.ErrSource
	}
	for _, row := range s.rows {
		if after.Kind != "" && (row.Kind < after.Kind || row.Kind == after.Kind && row.ID <= after.ID) {
			continue
		}
		if len(page.Rows) == storage.MaxLegacySourceRows {
			last := page.Rows[len(page.Rows)-1]
			page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
			break
		}
		page.Rows = append(page.Rows, row)
	}
	return page, ctx.Err()
}

func (s *editorSnapshotSource) Open(ctx context.Context, scope storage.Scope, epoch string, row privatePhotos.SnapshotRow) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.bodies[row.Kind+"/"+row.ID]
	if !ok || scope != s.state.Scope || epoch != s.state.Epoch {
		return nil, privatePhotos.ErrSource
	}
	s.opens++
	return io.NopCloser(bytes.NewReader(raw)), ctx.Err()
}

type editorLostACK struct {
	*mx.Client
	lost atomic.Bool
	puts atomic.Int32
}

func (p *editorLostACK) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	p.puts.Add(1)
	return p.Client.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
}

func (p *editorLostACK) AcknowledgeStorageFileLink(ctx context.Context, journal *offline.Store, job string, ref storage.Reference) error {
	err := p.Client.AcknowledgeStorageFileLink(ctx, journal, job, ref)
	if err == nil && p.lost.CompareAndSwap(false, true) {
		return errors.New("lost native editor content ACK")
	}
	return err
}

func TestPrivateEditorAgainstSynapseSavesAndReopensAfterLostACK(t *testing.T) {
	if os.Getenv("SYTUE_NATIVE_PHOTOS_EDITOR") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	origin, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_NATIVE_PHOTOS_EDITOR_URL"))
	if err != nil {
		t.Fatal(err)
	}
	const password = "artificial-disposable-photos-editor-password"
	register := func(name string) *mautrix.Client {
		client, err := mautrix.NewClient(origin.String(), "", "")
		if err != nil {
			t.Fatal(err)
		}
		matrixprooftest.Register(ctx, t, client, name, password, "REGISTRATION")
		return client
	}
	scope := storage.Scope{Tenant: "photos-native-editor", Subject: uuid.NewString(), Product: "photos"}
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
	raw := pngDe(bandas(40, 20))
	assetID := uuid.NewString()
	asset, _ := json.Marshal(model.AssetRow{ID: assetID, Tenant: scope.Tenant, Owner: scope.Subject, DriveID: "retained-original", Name: "editor.png", MediaType: "image/png", Bytes: int64(len(raw)), Width: 40, Height: 20, Orientation: 6, UploadedAt: time.Now().UTC()})
	digest := func(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
	media, _ := json.Marshal(model.MediaRow{ID: assetID, Tenant: scope.Tenant, Owner: scope.Subject, AssetID: assetID, Role: "original", Size: int64(len(raw)), SHA256: digest(raw), ContentType: "image/png"})
	source := &editorSnapshotSource{state: privatePhotos.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, bodies: map[string][]byte{"asset/" + assetID: asset, "media/" + assetID: media, "original/" + assetID: raw}}
	for _, kind := range []string{"asset", "media", "original"} {
		body := source.bodies[kind+"/"+assetID]
		source.rows = append(source.rows, privatePhotos.SnapshotRow{Kind: kind, ID: assetID, Size: int64(len(body)), SHA256: digest(body)})
	}
	blobs := &storagetest.CipherGateway{Product: "photos"}
	server := httptest.NewServer(blobs)
	defer server.Close()
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
	journal := newJournal()
	fault := &editorLostACK{Client: device.Client}
	repository := open(fault, journal)
	if err := repository.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	service := &servicioFalso{}
	mount := func(repository *privatePhotos.Repository) (*App, fyne.Window) {
		a, w := appDePrueba(t, service)
		private, err := a.cli.WithPrivateData(repository)
		if err != nil {
			t.Fatal(err)
		}
		a.cli = private
		a.Arrancar()
		return a, w
	}
	application, window := mount(repository)
	foto, err := repository.Photo(ctx, assetID)
	if err != nil {
		t.Fatal(err)
	}
	application.AbrirEditor(foto, "")
	if e := application.ed; e.cargando || e.original == nil || e.original.Bounds().Dx() != 20 || e.original.Bounds().Dy() != 40 {
		t.Fatal("actual private editor did not render its original preview")
	}
	window.Canvas().OnTypedRune()('r')
	wanted := application.ed.receta.Huella()
	tapSave := func(a *App) {
		t.Helper()
		header := a.ed.capa.dentro.(*fyne.Container).Objects[1].(*fyne.Container)
		button := header.Objects[5].(*ui.Boton)
		if button.Inerte {
			t.Fatal("real Save control was disabled")
		}
		test.Tap(button)
	}
	tapSave(application)
	if !fault.lost.Load() || application.ed.pendiente == nil || !application.ed.sucia() {
		t.Fatal("lost actual SDK ACK did not retain the editor attempt")
	}
	providerPuts := blobs.Snapshot().Puts
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
		t.Fatal("empty-journal fresh SDK could not open saved editor recipe", err)
	}
	visible, err := reader.LeerReceta(ctx, assetID)
	if err != nil || visible.Huella() != wanted {
		t.Fatal("fresh SDK saw a partial editor save", err)
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
	retry := &editorLostACK{Client: producer}
	retry.lost.Store(true)
	reopened := open(retry, journal)
	if err := reopened.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	application, window = mount(reopened)
	defer application.Cerrar()
	application.AbrirEditor(foto, "")
	if e := application.ed; e.cargando || e.pendiente == nil || e.base != api.RecetaVacia().Huella() || e.receta.Huella() != wanted || !e.sucia() {
		t.Fatal("SDK/UI reopen substituted the accepted recipe as a new base")
	}
	tapSave(application)
	if application.ed.sucia() || application.ed.pendiente != nil || retry.puts.Load() != 0 || blobs.Snapshot().Puts != providerPuts {
		t.Fatal("UI retry duplicated an accepted publication", retry.puts.Load())
	}
	if pending, err := reopened.Pending(ctx); err != nil || pending.Count != 0 {
		t.Fatal("UI retry retained acknowledged work", pending.Count, err)
	}
	var original bytes.Buffer
	if err := reader.Original(ctx, assetID, &original); err != nil || !bytes.Equal(original.Bytes(), raw) {
		t.Fatal("editor changed original bytes", err)
	}
	for _, variant := range model.Variantes() {
		body, err := reader.VariantDe(ctx, assetID, string(variant), wanted)
		if err != nil {
			t.Fatal("fresh SDK lost an editor rendition", variant, err)
		}
		encoded, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			t.Fatal(err)
		}
		picture, _, err := image.Decode(bytes.NewReader(encoded))
		clear(encoded)
		if err != nil || picture.Bounds().Dx() != 40 || picture.Bounds().Dy() != 20 {
			t.Fatal("fresh SDK did not receive complete rotated editor pixels", variant, err)
		}
	}
	source.mu.Lock()
	opens := source.opens
	source.mu.Unlock()
	if opens != 3 || len(service.vistas()) != 0 {
		t.Fatal("private editor reimported or reached legacy HTTP", opens, service.vistas())
	}
}
