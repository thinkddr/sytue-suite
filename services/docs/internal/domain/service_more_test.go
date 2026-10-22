// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// errFakeUnused lo devuelven los métodos del fake que ningún test ejercita, para
// no violar la regla nilnil (no devolver (nil, nil) en pares valor/error).
var errFakeUnused = errors.New("fake: método no usado en este test")

// fakeStore es un doble en memoria del contrato domain.Store, suficiente para
// ejercitar la lógica del servicio (autorización, validación, restauración) sin
// PostgreSQL. Solo implementa el comportamiento que los tests necesitan.
type fakeStore struct {
	doc      *Document
	perms    []Permission
	loadErr  error
	getErr   error
	loaded   *LoadedState
	appended []Op
	appendFn func(ops []Op) ([]SequencedOp, int64, error)

	version    *Version
	snapState  *State
	shareRes   *ShareResolution
	shareErr   error
	createdOn  *ShareLink
	starredArg *bool

	// registros de llamadas para aserciones.
	renameTitle string
	deleted     bool
}

func (f *fakeStore) CreateDocument(_ context.Context, p CreateDocumentParams) (*Document, bool, error) {
	return &Document{ID: "new", Kind: p.Kind, Title: p.Title, OwnerSub: p.OwnerSub, SourceRef: p.SourceRef}, true, nil
}

func (f *fakeStore) GetDocument(_ context.Context, _ string) (*Document, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.doc, nil
}

func (f *fakeStore) ListDocuments(_ context.Context, _ Subject, _ ListFilter) ([]Document, error) {
	if f.doc == nil {
		return nil, nil
	}
	return []Document{*f.doc}, nil
}

func (f *fakeStore) RenameDocument(_ context.Context, _, title string) error {
	f.renameTitle = title
	return nil
}

func (f *fakeStore) SoftDeleteDocument(_ context.Context, _ string) error {
	f.deleted = true
	return nil
}

func (f *fakeStore) SetStarred(_ context.Context, _, _ string, starred bool) error {
	f.starredArg = &starred
	return nil
}

func (f *fakeStore) AppendOps(_ context.Context, _ string, _ Subject, ops []Op, _ *int64) ([]SequencedOp, int64, error) {
	f.appended = append(f.appended, ops...)
	if f.appendFn != nil {
		return f.appendFn(ops)
	}
	out := make([]SequencedOp, len(ops))
	for i, op := range ops {
		out[i] = SequencedOp{Seq: int64(i + 1), Op: op}
	}
	return out, int64(len(ops)), nil
}

func (f *fakeStore) OpsSince(_ context.Context, _ string, _ int64, _ int) ([]SequencedOp, error) {
	return nil, nil
}

func (f *fakeStore) LoadState(_ context.Context, _ string) (*LoadedState, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	if f.loaded != nil {
		return f.loaded, nil
	}
	return &LoadedState{State: NewState(), Seq: 0}, nil
}

func (f *fakeStore) SaveSnapshot(_ context.Context, _ SaveSnapshotParams) (string, error) {
	return "snap-1", nil
}

func (f *fakeStore) GetSnapshot(_ context.Context, _ string) (*Snapshot, error) {
	return nil, errFakeUnused
}

func (f *fakeStore) ListPermissions(_ context.Context, _ string) ([]Permission, error) {
	return f.perms, nil
}
func (f *fakeStore) UpsertPermission(_ context.Context, _ string, _ Permission) error { return nil }
func (f *fakeStore) RevokePermission(_ context.Context, _ string, _ PrincipalKind, _ string) error {
	return nil
}

func (f *fakeStore) CreateShareLink(_ context.Context, docID, tokenHash, passwordHash, createdBy string, role Role, expiresAt *time.Time) (*ShareLink, error) {
	f.createdOn = &ShareLink{ID: "link-1", DocumentID: docID, Role: role, CreatedBy: createdBy, ExpiresAt: expiresAt, HasPassword: passwordHash != ""}
	return f.createdOn, nil
}

func (f *fakeStore) ListShareLinks(_ context.Context, _ string) ([]ShareLink, error) { return nil, nil }
func (f *fakeStore) RevokeShareLink(_ context.Context, _, _ string) error            { return nil }
func (f *fakeStore) PoliticaEnlaces(_ context.Context) (PoliticaEnlaces, error) {
	return PoliticaEnlaces{MaxDias: TechoEnlacesPorDefecto, PorDefecto: true}, nil
}

func (f *fakeStore) ResolveShareToken(_ context.Context, _ string) (*ShareResolution, error) {
	return f.shareRes, f.shareErr
}

func (f *fakeStore) CreateVersion(_ context.Context, docID, snapshotID, authorSub string, seq int64, label *string, auto bool) (*Version, error) {
	return &Version{ID: "v1", DocumentID: docID, SnapshotID: snapshotID, AuthorSub: authorSub, Seq: seq, Label: label, Auto: auto}, nil
}

func (f *fakeStore) ListVersions(_ context.Context, _ string, _ int) ([]Version, error) {
	return nil, nil
}

func (f *fakeStore) GetVersion(_ context.Context, _, _ string) (*Version, error) {
	return f.version, nil
}

func (f *fakeStore) SnapshotState(_ context.Context, _ string, _ func(ctx context.Context, blobKey string) (*State, error)) (*State, int64, error) {
	return f.snapState, 0, nil
}

func (f *fakeStore) CreateComment(_ context.Context, _ CreateCommentParams) (*Comment, error) {
	return nil, errFakeUnused
}

func (f *fakeStore) ListComments(_ context.Context, _ string, _ bool) ([]Comment, error) {
	return nil, nil
}
func (f *fakeStore) ResolveComment(_ context.Context, _, _, _ string, _ bool) error { return nil }
func (f *fakeStore) DeleteComment(_ context.Context, _, _, _ string, _ bool) error  { return nil }
func (f *fakeStore) RecordBlob(_ context.Context, b Blob) (*Blob, error)            { return &b, nil }

func (f *fakeStore) GetBlob(_ context.Context, _, _ string) (*Blob, error) {
	return nil, errFakeUnused
}

func (f *fakeStore) ListBlobs(_ context.Context, _ string) ([]Blob, error) { return nil, nil }

var _ Store = (*fakeStore)(nil)

func ownerSubject() Subject { return Subject{Sub: "owner"} }

func newOwnedDoc() *Document { return &Document{ID: "d1", OwnerSub: "owner", Kind: KindDocument} }

func TestServiceCreate(t *testing.T) {
	svc := NewService(&fakeStore{})
	ctx := context.Background()

	if _, _, err := svc.Create(ctx, ownerSubject(), Kind("bogus"), "T", ""); !errors.Is(err, ErrInvalidKind) {
		t.Errorf("kind inválido: got %v", err)
	}
	// Título vacío -> "Sin título".
	doc, _, err := svc.Create(ctx, ownerSubject(), KindDocument, "   ", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if doc.Title != "Sin título" {
		t.Errorf("título por defecto = %q", doc.Title)
	}
	// El origen llega al store tal cual, sin espacios de más: es la clave por la
	// que se reconoce el fichero al reabrirlo.
	vinculado, _, err := svc.Create(ctx, ownerSubject(), KindDocument, "Plan", "  drive:abc  ")
	if err != nil {
		t.Fatalf("Create con origen: %v", err)
	}
	if vinculado.SourceRef != "drive:abc" {
		t.Errorf("SourceRef = %q", vinculado.SourceRef)
	}
}

func TestServiceAuthorize(t *testing.T) {
	ctx := context.Background()

	// Owner alcanza cualquier rol mínimo.
	svc := NewService(&fakeStore{doc: newOwnedDoc()})
	if _, role, err := svc.Authorize(ctx, "d1", ownerSubject(), RoleOwner); err != nil || role != RoleOwner {
		t.Errorf("owner authorize: role=%q err=%v", role, err)
	}

	// Sin permisos: forbidden.
	svc = NewService(&fakeStore{doc: &Document{ID: "d1", OwnerSub: "someone"}})
	if _, _, err := svc.Authorize(ctx, "d1", Subject{Sub: "stranger"}, RoleViewer); !errors.Is(err, ErrForbidden) {
		t.Errorf("stranger: got %v, want forbidden", err)
	}

	// Rol insuficiente: viewer intenta editor.
	svc = NewService(&fakeStore{
		doc:   &Document{ID: "d1", OwnerSub: "someone"},
		perms: []Permission{{PrincipalKind: PrincipalUser, PrincipalID: "u", Role: RoleViewer}},
	})
	if _, _, err := svc.Authorize(ctx, "d1", Subject{Sub: "u"}, RoleEditor); !errors.Is(err, ErrForbidden) {
		t.Errorf("viewer->editor: got %v, want forbidden", err)
	}

	// Error del store al obtener el documento se propaga.
	svc = NewService(&fakeStore{getErr: ErrNotFound})
	if _, _, err := svc.Authorize(ctx, "d1", ownerSubject(), RoleViewer); !errors.Is(err, ErrNotFound) {
		t.Errorf("getErr: got %v", err)
	}
}

func TestServiceMutationsRequireRoles(t *testing.T) {
	ctx := context.Background()

	// Rename requiere editor; el owner lo tiene.
	fs := &fakeStore{doc: newOwnedDoc()}
	svc := NewService(fs)
	if err := svc.Rename(ctx, ownerSubject(), "d1", "  Nuevo  "); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if fs.renameTitle != "Nuevo" {
		t.Errorf("Rename recortado = %q", fs.renameTitle)
	}
	// Rename con título vacío falla antes de autorizar.
	if err := svc.Rename(ctx, ownerSubject(), "d1", "  "); !errors.Is(err, ErrTitleRequired) {
		t.Errorf("Rename vacío: got %v", err)
	}

	// Delete requiere owner.
	if err := svc.Delete(ctx, ownerSubject(), "d1"); err != nil || !fs.deleted {
		t.Errorf("Delete: err=%v deleted=%v", err, fs.deleted)
	}
	// Un no-owner no puede borrar.
	fs2 := &fakeStore{doc: &Document{ID: "d1", OwnerSub: "x"}}
	if err := NewService(fs2).Delete(ctx, Subject{Sub: "y"}, "d1"); !errors.Is(err, ErrForbidden) {
		t.Errorf("Delete no-owner: got %v", err)
	}

	// Star requiere viewer.
	if err := svc.Star(ctx, ownerSubject(), "d1", true); err != nil {
		t.Fatalf("Star: %v", err)
	}
	if fs.starredArg == nil || !*fs.starredArg {
		t.Error("Star no propagó el valor")
	}
}

func TestServiceApplyOps(t *testing.T) {
	ctx := context.Background()
	fs := &fakeStore{doc: newOwnedDoc()}
	svc := NewService(fs)
	v, _ := json.Marshal("x")
	ops := []Op{{Kind: OpInsert, Dot: Dot{Actor: "a", Counter: 1}, Value: v, Lamport: 1}}
	seqd, seq, err := svc.ApplyOps(ctx, ownerSubject(), "d1", ops, nil)
	if err != nil || seq != 1 || len(seqd) != 1 {
		t.Fatalf("ApplyOps: seqd=%d seq=%d err=%v", len(seqd), seq, err)
	}

	// Sin rol editor -> forbidden.
	fs2 := &fakeStore{
		doc:   &Document{ID: "d1", OwnerSub: "x"},
		perms: []Permission{{PrincipalKind: PrincipalUser, PrincipalID: "u", Role: RoleViewer}},
	}
	if _, _, err := NewService(fs2).ApplyOps(ctx, Subject{Sub: "u"}, "d1", ops, nil); !errors.Is(err, ErrForbidden) {
		t.Errorf("ApplyOps viewer: got %v", err)
	}
}

func TestServiceLoadFull(t *testing.T) {
	ctx := context.Background()

	// Estado inline: se devuelve tal cual.
	inline := NewState()
	fs := &fakeStore{doc: newOwnedDoc(), loaded: &LoadedState{State: inline, Seq: 5}}
	st, seq, err := NewService(fs).LoadFull(ctx, ownerSubject(), "d1", nil)
	if err != nil || seq != 5 || st != inline {
		t.Fatalf("LoadFull inline: seq=%d err=%v", seq, err)
	}

	// Estado en S3 sin hidratador -> error.
	fs = &fakeStore{doc: newOwnedDoc(), loaded: &LoadedState{BlobKey: "k", Seq: 3}}
	if _, _, err := NewService(fs).LoadFull(ctx, ownerSubject(), "d1", nil); err == nil {
		t.Error("LoadFull S3 sin hidratador debe fallar")
	}

	// Estado en S3 con hidratador -> ok.
	fs = &fakeStore{doc: newOwnedDoc(), loaded: &LoadedState{BlobKey: "k", Seq: 3, BaseSeq: 1}}
	hydrate := func(context.Context, string) (*State, error) { return NewState(), nil }
	st, seq, err = NewService(fs).LoadFull(ctx, ownerSubject(), "d1", hydrate)
	if err != nil || seq != 3 || st == nil {
		t.Fatalf("LoadFull S3: seq=%d err=%v", seq, err)
	}
}

func TestServiceSnapshotInline(t *testing.T) {
	ctx := context.Background()
	fs := &fakeStore{doc: newOwnedDoc(), loaded: &LoadedState{State: NewState(), Seq: 2}}
	ver, err := NewService(fs).Snapshot(ctx, ownerSubject(), "d1", nil, true, 0, nil, nil, nil)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if ver.SnapshotID != "snap-1" || ver.Seq != 2 || !ver.Auto {
		t.Errorf("Snapshot version inesperada: %+v", ver)
	}
}

func TestServiceRestoreVersion(t *testing.T) {
	ctx := context.Background()

	// target con un carácter que el estado actual no tiene -> Diff genera ops.
	target := NewState()
	v, _ := json.Marshal("A")
	_, _ = target.Apply(Op{Kind: OpInsert, Dot: Dot{Actor: "orig", Counter: 1}, Value: v, Lamport: 1})

	fs := &fakeStore{
		doc:       newOwnedDoc(),
		loaded:    &LoadedState{State: NewState(), Seq: 4},
		version:   &Version{ID: "v1", SnapshotID: "snap-1"},
		snapState: target,
	}
	res, err := NewService(fs).RestoreVersion(ctx, ownerSubject(), "d1", "v1", nil)
	if err != nil {
		t.Fatalf("RestoreVersion: %v", err)
	}
	if !res.Changed || len(res.Ops) == 0 {
		t.Errorf("RestoreVersion debía cambiar el documento: %+v", res)
	}

	// Restaurar a un estado idéntico -> Changed=false, sin ops.
	fs2 := &fakeStore{
		doc:       newOwnedDoc(),
		loaded:    &LoadedState{State: NewState(), Seq: 4},
		version:   &Version{ID: "v1", SnapshotID: "snap-1"},
		snapState: NewState(),
	}
	res, err = NewService(fs2).RestoreVersion(ctx, ownerSubject(), "d1", "v1", nil)
	if err != nil {
		t.Fatalf("RestoreVersion idéntico: %v", err)
	}
	if res.Changed || len(res.Ops) != 0 {
		t.Errorf("RestoreVersion idéntico no debía cambiar: %+v", res)
	}
}

func TestServiceShares(t *testing.T) {
	ctx := context.Background()

	// CreateShare rechaza rol owner y roles inválidos.
	svc := NewService(&fakeStore{doc: newOwnedDoc()})
	if _, err := svc.CreateShare(ctx, ownerSubject(), "d1", RoleOwner, nil, ""); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("CreateShare owner: got %v", err)
	}
	if _, err := svc.CreateShare(ctx, ownerSubject(), "d1", Role("nope"), nil, ""); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("CreateShare inválido: got %v", err)
	}

	// CreateShare feliz: devuelve el token en claro.
	link, err := svc.CreateShare(ctx, ownerSubject(), "d1", RoleViewer, nil, "")
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	if link.Token == "" {
		t.Error("CreateShare debe exponer el token en claro")
	}

	// ResolveShare hashea el token antes de consultar.
	res := &ShareResolution{DocumentID: "d1", Role: RoleViewer}
	svc = NewService(&fakeStore{shareRes: res})
	got, err := svc.ResolveShare(ctx, "plaintext-token")
	if err != nil || got != res {
		t.Fatalf("ResolveShare: got=%v err=%v", got, err)
	}
}

func TestServiceList(t *testing.T) {
	ctx := context.Background()
	doc := newOwnedDoc()
	doc.StarredBy = []string{"owner"}
	docs, err := NewService(&fakeStore{doc: doc}).List(ctx, ownerSubject(), ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(docs) != 1 || !docs[0].Starred {
		t.Errorf("List debía marcar Starred para el owner: %+v", docs)
	}
}

func TestTokenRoundTrip(t *testing.T) {
	tok, err := NewToken()
	if err != nil || len(tok) != 64 {
		t.Fatalf("NewToken: %q err=%v", tok, err)
	}
	if HashToken(tok) == tok {
		t.Error("HashToken no debe devolver el token en claro")
	}
	// Determinista: mismo input -> mismo hash (valor sha-256 hex conocido de "x").
	const wantX = "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"
	if HashToken("x") != wantX {
		t.Errorf("HashToken(\"x\") = %q, want %q", HashToken("x"), wantX)
	}
}
