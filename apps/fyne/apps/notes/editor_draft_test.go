// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestEditorPersistsLastKeystrokeBeforeAutosaveAndDetach(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>hola</p>", UpdatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)})
	p.Almacen = almacenDePrueba(t)
	p.Yo = func() string { return "alice" }
	p.Org = func() string { return "acme" }
	n, _ := p.nota("n1")
	p.abrirEditor(n)
	teclear(p.edicion.editor.zona, " mundo")
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows, err := p.Almacen.List("alice/acme", espacioBorradores)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("last editor keystroke not retained in encrypted journal: %d records", len(rows))
	}
	var d struct {
		HTML      string        `json:"html"`
		FromInput bool          `json:"body_from_input"`
		Input     api.NoteInput `json:"input"`
		NoteID    string        `json:"note_id"`
	}
	if err := json.Unmarshal(rows[0].Value, &d); err != nil {
		t.Fatal(err)
	}
	if d.HTML != "" || !d.FromInput || d.Input.Content != "<p>hola mundo</p>" || d.NoteID != "n1" {
		t.Fatal("editor draft lost identity or exact content", d)
	}
}

func TestEditorDraftAckRetainsNewerRevisionAndOtherOwner(t *testing.T) {
	p := &Pantalla{Almacen: almacenDePrueba(t), Yo: func() string { return "alice" }, Org: func() string { return "acme" }}
	old := borradorEdicion{Version: 1, NoteID: "n1", NotebookID: "c1", Revision: "r1", HTML: "uno", ExpectedUpdatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	newer := old
	newer.Revision = "r2"
	newer.HTML = "dos"
	id := prefijoEdicion + "fixture"
	if err := p.escribirEdicion(t.Context(), id, newer); err != nil {
		t.Fatal(err)
	}
	if err := p.Almacen.Write("bob/acme", espacioBorradores, id, old); err != nil {
		t.Fatal(err)
	}
	updated := api.Note{ID: "n1", NotebookID: "c1", UpdatedAt: old.ExpectedUpdatedAt.Add(time.Second)}
	if err := p.confirmarEdicion(t.Context(), id, old, updated); err != nil {
		t.Fatal(err)
	}
	got, found, err := p.leerEdicion(id)
	if err != nil || !found || got.HTML != "dos" || got.Revision != "r2" || !got.ExpectedUpdatedAt.Equal(updated.UpdatedAt) {
		t.Fatal("old ACK removed or failed to rebase newer keystrokes", got, found, err)
	}
	if err := p.confirmarEdicion(t.Context(), id, old, api.Note{ID: "foreign", NotebookID: "c1"}); err == nil {
		t.Fatal("foreign ACK accepted")
	}
	if err := p.enviarEdicion(t.Context(), id, func(_ context.Context, d borradorEdicion) (api.Note, error) {
		if d.Input.ExpectedUpdatedAt == nil || !d.Input.ExpectedUpdatedAt.Equal(updated.UpdatedAt) {
			t.Fatal("replay lost stored optimistic precondition")
		}
		return updated, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, found, err = p.leerEdicion(id)
	if err != nil || found {
		t.Fatal("exact acknowledged draft was retained", found, err)
	}
	var foreign borradorEdicion
	if ok, err := p.Almacen.Read("bob/acme", espacioBorradores, id, &foreign); err != nil || !ok || foreign.Revision != "r1" {
		t.Fatal("other account's source changed", ok, err)
	}
}

func TestEditorDraftFailedReplayAndMalformedSourceAreRetained(t *testing.T) {
	p := &Pantalla{Almacen: almacenDePrueba(t), Yo: func() string { return "alice" }}
	d := borradorEdicion{Version: 1, NoteID: "n1", NotebookID: "c1", Revision: "r1", HTML: "pendiente"}
	id := prefijoEdicion + "conflict"
	if err := p.escribirEdicion(t.Context(), id, d); err != nil {
		t.Fatal(err)
	}
	conflict := errors.New("newer remote version")
	if err := p.enviarEdicion(t.Context(), id, func(context.Context, borradorEdicion) (api.Note, error) { return api.Note{}, conflict }); !errors.Is(err, conflict) {
		t.Fatal(err)
	}
	if got, found, err := p.leerEdicion(id); err != nil || !found || got.HTML != d.HTML {
		t.Fatal("failed replay lost source", got, found, err)
	}
	if err := p.Almacen.Write(p.claveAlmacen(), espacioBorradores, id, map[string]any{"version": 99, "html": "unrecognized"}); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := p.enviarEdicion(t.Context(), id, func(context.Context, borradorEdicion) (api.Note, error) { called = true; return api.Note{}, nil }); err == nil || called {
		t.Fatal("malformed source sent or acknowledged", called, err)
	}
	rows, err := p.Almacen.List(p.claveAlmacen(), espacioBorradores)
	if err != nil || len(rows) != 1 {
		t.Fatal("malformed source erased", len(rows), err)
	}
}

func TestLegacyDraftWithClearEncryptedLinksNeverSendsOrDisappears(t *testing.T) {
	p := &Pantalla{Almacen: almacenDePrueba(t), Yo: func() string { return "alice" }}
	d := borradorEdicion{Version: 1, NoteID: "n1", NotebookID: "c1", Revision: "r1", HTML: "pending", Input: api.NoteInput{ContentCipher: "encv1:synthetic", CipherVersion: 2, Links: []api.NoteLink{{TargetKind: "docs_document", TargetID: "private"}}}}
	id := prefijoEdicion + "unsafe"
	if err := p.escribirEdicion(t.Context(), id, d); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err := p.enviarEdicion(t.Context(), id, func(context.Context, borradorEdicion) (api.Note, error) { calls++; return api.Note{}, nil })
	if !errors.Is(err, api.ErrDatosCifradosNotas) || calls != 0 {
		t.Fatal("unsafe draft was sent", err, calls)
	}
	got, found, err := p.leerEdicion(id)
	if err != nil || !found || got.HTML != d.HTML || len(got.Input.Links) != 1 {
		t.Fatal("rejected draft was lost", found, err)
	}
}

func TestUnsafeEncryptedEditRemainsEncryptedAndRestorable(t *testing.T) {
	key := bytes.Repeat([]byte{19}, 32)
	in := api.NoteInput{Kind: "text", Content: "<p>original</p>"}
	if err := api.CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: in.ContentCipher, Data: in.Data, Links: []api.NoteLink{{TargetKind: "docs_document", TargetID: "private"}}}
	p, server := pantallaConServicio(t, nil, n)
	p.Almacen = almacenDePrueba(t)
	p.Yo = func() string { return "alice" }
	p.activo.Cifrado, p.clave = true, key
	p.notas = p.deNotas([]api.Note{n})
	p.abrirEditor(p.notas[0])
	previous := p.edicion.html
	teclear(p.edicion.editor.zona, " secret")
	updated := p.edicion.editor.HTML()
	if updated == previous || !p.edicion.bloqueoBorrador || len(p.avisos.Avisos()) == 0 {
		t.Fatal("pending change or blocked warning missing")
	}
	id := p.edicion.borradorID
	p.guardarEdicion(p.edicion)
	draft, found, err := p.leerEdicion(id)
	if err != nil || !found || draft.Version != 3 || !draft.Blocked || draft.EditorHTML() != updated {
		t.Fatal("blocked change was not durably retained", found, err)
	}
	p.cerrarEditor()
	if len(server.rutas()) != 0 || p.notas[0].Crudo.ContentCipher != n.ContentCipher {
		t.Fatal("rejected change sent or changed the original")
	}
	p.abrirEditor(p.notas[0])
	if p.edicion.editor.HTML() != updated || !p.edicion.bloqueoBorrador {
		t.Fatal("reopening lost pending blocked text")
	}
}

func TestBlockedDraftWithoutLinksCannotReplay(t *testing.T) {
	p := &Pantalla{Almacen: almacenDePrueba(t), Yo: func() string { return "alice" }}
	id := prefijoEdicion + "blocked-no-links"
	d := borradorEdicion{Version: 3, Blocked: true, NoteID: "n1", NotebookID: "c1", Revision: "r1", HTML: "private pending", Input: api.NoteInput{CipherVersion: 2, ContentCipher: "encv1:synthetic"}}
	if err := p.escribirEdicion(t.Context(), id, d); err != nil {
		t.Fatal(err)
	}
	called := false
	err := p.enviarEdicion(t.Context(), id, func(context.Context, borradorEdicion) (api.Note, error) { called = true; return api.Note{}, nil })
	got, found, readErr := p.leerEdicion(id)
	if !errors.Is(err, api.ErrDatosCifradosNotas) || called || readErr != nil || !found || got.EditorHTML() != d.HTML {
		t.Fatal("blocked draft was replayed or lost", err, readErr)
	}
}

func TestPrivateBlockedEditSurvivesDetachAndIsCounted(t *testing.T) {
	key := bytes.Repeat([]byte{19}, 32)
	in := api.NoteInput{Kind: "text", Content: "<p>original</p>"}
	if err := api.CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: in.ContentCipher, Data: in.Data, Links: []api.NoteLink{{TargetKind: "docs_document", TargetID: "private"}}}
	p := draftRestoreScreen(t, true, nil)
	p.activo.Cifrado, p.clave = true, bytes.Clone(key)
	p.notas = p.deNotas([]api.Note{n})
	p.abrirEditor(p.notas[0])
	teclear(p.edicion.editor.zona, " pending")
	id := p.edicion.borradorID
	expected := p.edicion.html
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
	if pending, err := p.vida().work.Pending(t.Context()); err != nil || pending.Count != 1 {
		t.Fatal("blocked edit is not counted", pending, err)
	}
	draft, found, err := p.leerEdicion(id)
	if err != nil || !found || !draft.Blocked || draft.Version != 3 || draft.EditorHTML() != expected {
		t.Fatal("blocked edit not restored after detach", found, err)
	}
}

func TestUnsafeEditLosingStoreKeepsRetirementBarrier(t *testing.T) {
	key := bytes.Repeat([]byte{19}, 32)
	in := api.NoteInput{Kind: "text", Content: "<p>original</p>"}
	if err := api.CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: in.ContentCipher, Data: in.Data, Links: []api.NoteLink{{TargetKind: "docs_document", TargetID: "private"}}}
	p, _ := pantallaConServicio(t, nil, n)
	p.Almacen = almacenDePrueba(t)
	p.Yo = func() string { return "alice" }
	p.activo.Cifrado, p.clave = true, key
	p.notas = p.deNotas([]api.Note{n})
	p.abrirEditor(p.notas[0])
	p.Almacen = nil
	teclear(p.edicion.editor.zona, " pending")
	s := p.edicion
	p.ReiniciarSesion()
	if p.edicion != s || !s.bloqueoBorrador || s.html != "<p>original pending</p>" {
		t.Fatal("session reset discarded an unpreserved edit")
	}
	p.Cerrar()
	if err := p.Join(t.Context()); !errors.Is(err, offline.ErrEncryptedStore) {
		t.Fatal("retirement released an unpreserved edit", err)
	}
}

func TestCipherFailureKeepsBlockedDraftRecoverable(t *testing.T) {
	key := bytes.Repeat([]byte{19}, 32)
	in := api.NoteInput{Kind: "text", Content: "<p>original</p>"}
	if err := api.CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: in.ContentCipher, Data: in.Data}
	p, server := pantallaConServicio(t, nil, n)
	p.Almacen = almacenDePrueba(t)
	p.Yo = func() string { return "alice" }
	p.activo.Cifrado, p.clave = true, key
	p.notas = p.deNotas([]api.Note{n})
	p.abrirEditor(p.notas[0])
	// The raw envelope becomes unreadable after the editor has opened.
	p.notas[0].Crudo.ContentCipher = "invalid-cipher"
	teclear(p.edicion.editor.zona, " pending")
	id, expected := p.edicion.borradorID, p.edicion.html
	draft, found, err := p.leerEdicion(id)
	if err != nil || !found || !draft.Blocked || draft.Version != 3 || draft.EditorHTML() != expected {
		t.Fatal("cipher error did not preserve the exact blocked edit", found, err)
	}
	if !p.PrepararRetirada() || len(server.rutas()) != 0 {
		t.Fatal("durable blocked draft did not allow a safe retirement")
	}
}

func TestWriteFailureKeepsEditorUntilDurableRetry(t *testing.T) {
	key := bytes.Repeat([]byte{19}, 32)
	in := api.NoteInput{Kind: "text", Content: "<p>original</p>"}
	if err := api.CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: in.ContentCipher, Data: in.Data, Links: []api.NoteLink{{TargetKind: "docs_document", TargetID: "private"}}}
	dir := t.TempDir()
	journal, err := offline.OpenWithSecrets(dir, new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	p, server := pantallaConServicio(t, nil, n)
	p.Almacen = journal
	p.Yo = func() string { return "alice" }
	p.Org = func() string { return "acme" }
	p.activo.Cifrado, p.clave = true, key
	p.notas = p.deNotas([]api.Note{n})
	p.abrirEditor(p.notas[0])
	// The actual encrypted store must fail to create the owner's directory.
	owner := sha256.Sum256([]byte("alice/acme"))
	obstruction := filepath.Join(dir, hex.EncodeToString(owner[:16]))
	if err := os.WriteFile(obstruction, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	teclear(p.edicion.editor.zona, " pending")
	s := p.edicion
	if s.errorBorrador == nil || s.html != "<p>original pending</p>" || p.PrepararRetirada() || p.edicion != s {
		t.Fatal("write failure allowed exit or lost text")
	}
	if err := os.Remove(obstruction); err != nil {
		t.Fatal(err)
	}
	if !p.PrepararRetirada() || len(server.rutas()) != 0 {
		t.Fatal("durable retry failed or sent the blocked input")
	}
	draft, found, err := p.leerEdicion(s.borradorID)
	if err != nil || !found || draft.EditorHTML() != s.html || !draft.Blocked {
		t.Fatal("retry did not restore the exact encrypted journal entry", found, err)
	}
}

func TestInvalidUTF8IsNotSilentlyReplacedInEditorDraft(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>original</p>"})
	p.Almacen = almacenDePrueba(t)
	p.Yo = func() string { return "alice" }
	p.notas = p.deNotas([]api.Note{{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>original</p>"}})
	p.abrirEditor(p.notas[0])
	s := p.edicion
	s.editor.PegarTexto(string([]byte{0xff}))
	invalid := s.editor.HTML()
	if !bytes.Contains([]byte(invalid), []byte{0xff}) || s.html != invalid || !s.bloqueoBorrador || !errors.Is(s.errorBorrador, errBorradorUTF8) {
		t.Fatal("invalid UTF-8 was normalized or not retained for correction")
	}
	if p.PrepararRetirada() || p.edicion != s {
		t.Fatal("retirement lost invalid editor bytes")
	}
	rows, err := p.Almacen.List(p.claveAlmacen(), espacioBorradores)
	if err != nil || len(rows) != 0 {
		t.Fatal("invalid text was silently serialized", len(rows), err)
	}
}

func TestPrivateUnadmittedEditRetriesBeforeRetirement(t *testing.T) {
	key := bytes.Repeat([]byte{19}, 32)
	in := api.NoteInput{Kind: "text", Content: "<p>original</p>"}
	if err := api.CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: in.ContentCipher, Data: in.Data, Links: []api.NoteLink{{TargetKind: "docs_document", TargetID: "private"}}}
	journal := almacenDePrueba(t)
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	admit := false
	work, err := notas.NewLocalWork(t.Context(), scope, journal, func(context.Context) (func(), error) {
		if !admit {
			return nil, offline.ErrWorkLease
		}
		return func() {}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	p, err := NuevaPantallaPrivada(t.Context(), work, trInterno(t), "es", privatePresenterData{scope: scope}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.activo.Cifrado, p.clave = true, bytes.Clone(key)
	p.notas = p.deNotas([]api.Note{n})
	p.abrirEditor(p.notas[0])
	teclear(p.edicion.editor.zona, " pending")
	s := p.edicion
	if s == nil || s.borrador == nil || !s.bloqueoBorrador || s.html != "<p>original pending</p>" {
		t.Fatal("failed admission discarded its source")
	}
	if p.PrepararRetirada() || p.edicion != s {
		t.Fatal("retirement passed a failed lease")
	}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	go func() {
		finished <- journal.Exclusive(t.Context(), work.Owner(), espacioBorradores, s.borradorID, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	admit = true
	if p.PrepararRetirada() || s.durable == nil {
		t.Fatal("retirement passed before the admitted source was durable")
	}
	unblock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	<-s.durable.Done()
	if !p.PrepararRetirada() {
		t.Fatal("retirement stayed blocked after durable retry")
	}
	draft, found, err := p.leerEdicion(s.borradorID)
	if err != nil || !found || draft.EditorHTML() != s.html || !draft.Blocked {
		t.Fatal("retry did not preserve exact source", found, err)
	}
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestUnsafeEncryptedEditWithoutStoreIsNotAdmitted(t *testing.T) {
	key := bytes.Repeat([]byte{19}, 32)
	in := api.NoteInput{Kind: "text", Content: "<p>original</p>"}
	if err := api.CifrarContenidoNota(key, &in); err != nil {
		t.Fatal(err)
	}
	n := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: in.ContentCipher, Data: in.Data, Links: []api.NoteLink{{TargetKind: "docs_document", TargetID: "private"}}}
	p, server := pantallaConServicio(t, nil, n)
	p.activo.Cifrado, p.clave = true, key
	p.notas = p.deNotas([]api.Note{n})
	p.abrirEditor(p.notas[0])
	if p.edicion != nil || len(p.avisos.Avisos()) == 0 || len(server.rutas()) != 0 {
		t.Fatal("editing admitted without encrypted storage or warning")
	}
	p.ReiniciarSesion()
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestEditorDraftRecoversExactTextAfterNewPresenter(t *testing.T) {
	original := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>original</p>", UpdatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	p, _ := pantallaConServicio(t, nil, original)
	p.Almacen = almacenDePrueba(t)
	p.Yo = func() string { return "alice" }
	p.Org = func() string { return "acme" }
	n, _ := p.nota("n1")
	p.abrirEditor(n)
	teclear(p.edicion.editor.zona, " recuperado")
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
	restarted, _ := pantallaConServicio(t, nil, original)
	restarted.Almacen = p.Almacen
	restarted.Yo = func() string { return "alice" }
	restarted.Org = func() string { return "acme" }
	n, _ = restarted.nota("n1")
	restarted.abrirEditor(n)
	if got := restarted.edicion.editor.HTML(); got != "<p>original recuperado</p>" {
		t.Fatal("new presenter did not recover exact local draft", got)
	}
	current, _ := restarted.nota("n1")
	if current.Crudo.Content != original.Content {
		t.Fatal("draft restoration overwrote the remote original")
	}
	restarted.Cerrar()
	if err := restarted.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestEditorDraftVersion1StillRestoresExactLegacyHTML(t *testing.T) {
	original := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>original</p>"}
	p, _ := pantallaConServicio(t, nil, original)
	p.Almacen = almacenDePrueba(t)
	p.Yo = func() string { return "alice" }
	p.Org = func() string { return "acme" }
	legacy := borradorEdicion{Version: 1, NoteID: "n1", NotebookID: "c1", Revision: "old-installation", HTML: "<p>legacy / 漢字</p>", Input: api.NoteInput{Content: "<p>legacy / 漢字</p>"}}
	if err := p.Almacen.Write("alice/acme", espacioBorradores, prefijoEdicion+"legacy", legacy); err != nil {
		t.Fatal(err)
	}
	n, _ := p.nota("n1")
	p.abrirEditor(n)
	if html := p.edicion.editor.HTML(); html != legacy.HTML {
		t.Fatal("version1 restoration changed original HTML", html)
	}
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestEditorDraftVersion2KeepsOptionalNotebookCipherAndRestoresAfterRestart(t *testing.T) {
	for _, mode := range []struct {
		name    string
		private bool
	}{{"legacy", false}, {"admitted-queue", true}} {
		t.Run(mode.name, func(t *testing.T) { exerciseOptionalNotebookDraftRestore(t, mode.private) })
	}
}

func draftRestoreScreen(t *testing.T, private bool, journal *offline.Store) *Pantalla {
	t.Helper()
	if !private {
		p, _ := pantallaConServicio(t, nil)
		return p
	}
	if journal == nil {
		journal = almacenDePrueba(t)
	}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	work, err := notas.NewLocalWork(t.Context(), scope, journal, func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	p, err := NuevaPantallaPrivada(t.Context(), work, trInterno(t), "es", privatePresenterData{scope: scope}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func exerciseOptionalNotebookDraftRestore(t *testing.T, private bool) {
	key := bytes.Repeat([]byte{19}, 32)
	defer clear(key)
	input := api.NoteInput{Kind: "text", Title: "private title", Content: "<p>original</p>"}
	if err := api.CifrarContenidoNota(key, &input); err != nil {
		t.Fatal(err)
	}
	original := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: input.ContentCipher, Data: input.Data, UpdatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	p := draftRestoreScreen(t, private, nil)
	if !private {
		p.Almacen = almacenDePrueba(t)
	}
	p.Yo = func() string { return "alice" }
	p.Org = func() string { return "acme" }
	p.activo.Cifrado = true
	p.clave = bytes.Clone(key)
	p.notas = p.deNotas([]api.Note{original})
	n, _ := p.nota("n1")
	p.abrirEditor(n)
	teclear(p.edicion.editor.zona, " pendiente")
	id := p.edicion.borradorID
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
	draft, found, err := p.leerEdicion(id)
	if err != nil || !found || draft.Version != 2 || draft.FromInput || draft.Input.Content != "" || draft.Input.ContentCipher == "" {
		t.Fatal("optional notebook draft lost its cipher", found, err)
	}
	expected := "<p>original pendiente</p>"
	cleartext, err := api.DesempaquetarNota(key, draft.Input.ContentCipher)
	if err != nil || cleartext.Content != expected || draft.HTML != expected {
		t.Fatal("cipher/plain restoration sources differ", err)
	}
	restarted := draftRestoreScreen(t, private, p.Almacen)
	restarted.Almacen = p.Almacen
	restarted.Yo = func() string { return "alice" }
	restarted.Org = func() string { return "acme" }
	restarted.activo.Cifrado = true
	restarted.clave = bytes.Clone(key)
	restarted.notas = restarted.deNotas([]api.Note{original})
	n, _ = restarted.nota("n1")
	restarted.abrirEditor(n)
	if got := restarted.edicion.editor.HTML(); got != expected {
		t.Fatal("encrypted notebook lost pending edit on restart", got)
	}
	restarted.Cerrar()
	if err := restarted.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestEditorDraftVersion2RejectsAmbiguousCompactCipher(t *testing.T) {
	d := borradorEdicion{Version: 2, NoteID: "n1", NotebookID: "c1", Revision: "r1", FromInput: true, Input: api.NoteInput{Content: "body"}}
	if !d.Valid() {
		t.Fatal("exact compact body rejected")
	}
	d.HTML = "different"
	if d.Valid() {
		t.Fatal("two representations accepted as exact")
	}
	d.HTML = ""
	d.Input.ContentCipher = "encrypted"
	if d.Valid() {
		t.Fatal("ciphertext advertised as directly reconstructible HTML")
	}
}

func TestPrivateEditorWaitsForDurableDraftBeforeMarkingOrSending(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	journal := almacenDePrueba(t)
	work, err := notas.NewLocalWork(t.Context(), scope, journal, func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	callbacks := make(chan func(), 8)
	p, err := NuevaPantallaPrivada(t.Context(), work, trInterno(t), "es", privatePresenterData{scope: scope}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.lifetime.dispatch = func(fn func()) { callbacks <- fn }
	p.activo = DeCuaderno(api.Notebook{ID: "c1", OwnerSub: "alice", Visibility: "personal"})
	p.notas = DeNotas([]api.Note{{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>original</p>"}})
	s := &sesionEdicion{nota: "n1", editor: &EditorNota{doc: Leer("<p>new</p>")}, html: "<p>new</p>", guardado: "<p>original</p>", borradorID: "edicion:blocked"}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	go func() {
		finished <- journal.Exclusive(t.Context(), work.Owner(), espacioBorradores, s.borradorID, func() error { close(entered); <-release; return nil })
	}()
	<-entered
	if err := p.conservarEdicion(s, s.html); err != nil {
		t.Fatal(err)
	}
	p.guardarEdicion(s)
	if s.guardado != "<p>original</p>" || !s.esperaDurable {
		t.Fatal("RAM-only draft advanced the submitted revision", s.guardado, s.esperaDurable)
	}
	if durable, err := s.durable.Status(); durable || err != nil {
		t.Fatal(durable, err)
	}
	// Detach invalidates the observer, while Join must still finish its durable source.
	p.Cerrar()
	unblock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
	for len(callbacks) > 0 {
		(<-callbacks)()
	}
	if pending, err := work.Pending(t.Context()); err != nil || pending.Count != 1 {
		t.Fatal("accepted source not counted after durable handoff", pending, err)
	}
	if s.guardado != "<p>original</p>" {
		t.Fatal("detached durability callback revived a sender")
	}
	var stored borradorEdicion
	if found, err := journal.Read(work.Owner(), espacioBorradores, s.borradorID, &stored); err != nil || !found || stored.EditorHTML() != s.html {
		t.Fatal("detach lost accepted editor source", found, err)
	}
}

func TestPrivateEditorReopensAcceptedRAMBeforeOlderWALAndRetainsRevision(t *testing.T) {
	p := draftRestoreScreen(t, true, nil)
	t.Cleanup(p.Cerrar)
	work := p.vida().work
	callbacks := make(chan func(), 128)
	p.lifetime.dispatch = func(fn func()) { callbacks <- fn }
	p.activo = DeCuaderno(api.Notebook{ID: "c1", OwnerSub: "alice", Visibility: "personal"})
	original := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>original</p>", UpdatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	p.notas = DeNotas([]api.Note{original})
	v1 := borradorEdicion{Version: 2, NoteID: "n1", NotebookID: "c1", Revision: "r1", FromInput: true, Input: api.NoteInput{Kind: "text", Content: "<p>old WAL</p>"}, ExpectedUpdatedAt: original.UpdatedAt}
	id := "edicion:reopen"
	if err := p.Almacen.Write(work.Owner(), espacioBorradores, id, v1); err != nil {
		t.Fatal(err)
	}
	n, _ := p.nota("n1")
	p.abrirEditor(n)
	first := p.edicion
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	go func() {
		finished <- p.Almacen.Exclusive(t.Context(), work.Owner(), espacioBorradores, id, func() error { close(entered); <-release; return nil })
	}()
	<-entered
	teclear(first.editor.zona, " v2")
	revision, receipt := first.borrador.Revision, first.durable
	p.cerrarEditor()
	p.abrirEditor(n)
	reopened := p.edicion
	if reopened == first || reopened.editor.HTML() != "<p>old WAL v2</p>" || reopened.borradorID != id || reopened.borrador.Revision != revision || reopened.durable != receipt {
		t.Fatal("reopen displayed old WAL or replaced the accepted revision", reopened.editor.HTML())
	}
	// A late event from the old editor cannot replace the accepted source.
	p.alEscribir(first, "<p>stale detached editor</p>")
	if _, pending, _, found := work.PendingEditor("n1", "c1"); !found || pending.Revision != revision {
		t.Fatal("detached editor replaced reopened source", found)
	}
	teclear(reopened.editor.zona, " v3")
	latest := reopened.borrador.Revision
	if latest == revision {
		t.Fatal("new edit lost its own revision")
	}
	p.cerrarEditor()
	unblock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	<-reopened.durable.Done()
	if durable, err := receipt.Status(); !durable || err != nil {
		t.Fatal("older coalesced receipt did not finish", durable, err)
	}
	// A sender awakened by any old receipt reads the latest durable record,
	// never the HTML retained by that old editor session.
	sends := 0
	if err := p.enviarEdicionConfirmada(t.Context(), id, func(ctx context.Context, d borradorEdicion, acknowledge func(context.Context, api.Note) error) (api.Note, error) {
		sends++
		if d.Revision != latest || d.EditorHTML() != "<p>old WAL v2 v3</p>" {
			t.Fatal("old receipt published an older editor body", d.Revision, d.EditorHTML())
		}
		updated := api.Note{ID: "n1", NotebookID: "c1", UpdatedAt: original.UpdatedAt.Add(time.Second)}
		return updated, acknowledge(ctx, updated)
	}); err != nil {
		t.Fatal(err)
	}
	if sends != 1 {
		t.Fatal("latest durable edit was not sent", sends)
	}
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
	for len(callbacks) > 0 {
		(<-callbacks)()
	}
	if found, err := p.Almacen.Read(work.Owner(), espacioBorradores, id, &borradorEdicion{}); err != nil || found {
		t.Fatal("exact latest ACK left an older draft", found, err)
	}
}
