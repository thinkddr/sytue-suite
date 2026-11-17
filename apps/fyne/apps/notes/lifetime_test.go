// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPresenterJoinCancelsAndWaitsForAdmittedCleanup(t *testing.T) {
	p := &Pantalla{Plazo: time.Minute}
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	if !p.iniciar(func() {
		ctx, cancel := p.ctx()
		defer cancel()
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
	}) {
		t.Fatal("work was not admitted")
	}
	<-entered
	p.Cerrar()
	<-canceled
	expired, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := p.Join(expired); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("join skipped admitted cleanup", err)
	}
	if p.iniciar(func() { t.Error("retired presenter admitted work") }) {
		t.Fatal("admission reopened after detach")
	}
	close(release)
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
	if p.vigente() {
		t.Fatal("retired presenter can be reused")
	}
}

func TestPresenterQueuedCallbacksCannotReviveAnotherLife(t *testing.T) {
	queue := make(chan func(), 4)
	p := &Pantalla{lifetime: vidaPantalla{dispatch: func(f func()) { queue <- f }, dispatchWait: func(f func()) { queue <- f }}}
	calls := 0
	p.enHilo(func() { calls++ })
	p.enHiloEsperar(func() { calls++ })
	p.Cerrar()
	for len(queue) > 0 {
		(<-queue)()
	}
	p.enHilo(func() { calls++ })
	if calls != 0 || len(queue) != 0 {
		t.Fatal("a retired callback touched the UI", calls, len(queue))
	}
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPresenterParentCancellationStopsRequestsAndAdmissions(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	p := &Pantalla{lifetime: vidaPantalla{parent: parent}, Plazo: time.Minute}
	ctx, done := p.ctx()
	defer done()
	cancel()
	<-ctx.Done()
	if p.iniciar(func() { t.Error("canceled parent admitted work") }) {
		t.Fatal("work admitted after resource lifetime ended")
	}
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPresenterWatcherStopsWithoutWaitingForThirtySecondRetry(t *testing.T) {
	var dispatched atomic.Int32
	p := &Pantalla{Almacen: almacenDePrueba(t), red: &offline.Monitor{}, lifetime: vidaPantalla{dispatch: func(func()) { dispatched.Add(1) }}}
	p.vigilarCola()
	p.Cerrar()
	deadline, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := p.Join(deadline); err != nil {
		t.Fatal("periodic capture worker survived detach", err)
	}
	p.red.Report(offline.Offline)
	p.red.Report(offline.Online)
	if dispatched.Load() != 0 {
		t.Fatal("retired connectivity callback touched UI")
	}
}

func TestPresenterResumeCannotRunQueuedCallbacksOfCanceledPhase(t *testing.T) {
	journal := almacenDePrueba(t)
	work, err := notas.NewLocalWork(t.Context(), storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}, journal, func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	queue := make(chan func(), 4)
	p := &Pantalla{lifetime: vidaPantalla{work: work, dispatch: func(fn func()) { queue <- fn }}, Almacen: journal}
	calls := 0
	p.enHilo(func() { calls++ })
	if err := work.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := work.Resume(); err != nil {
		t.Fatal(err)
	}
	(<-queue)()
	if calls != 0 {
		t.Fatal("old UI callback survived drain/resume")
	}
	p.enHilo(func() { calls++ })
	(<-queue)()
	if calls != 1 {
		t.Fatal("new phase could not update UI", calls)
	}
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

type privatePresenterData struct {
	api.NotesData
	scope storage.Scope
}

func (d privatePresenterData) PrivateStorageScope() storage.Scope { return d.scope }

func TestPrivatePresenterKeepsCapturedOwnerAndRejectsForeignData(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	journal := almacenDePrueba(t)
	work, err := notas.NewLocalWork(t.Context(), scope, journal, func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	p, err := NuevaPantallaPrivada(t.Context(), work, trInterno(t), "es", privatePresenterData{scope: scope}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Yo = func() string { return "bob" }
	p.Org = func() string { return "elsewhere" }
	p.guardarBorrador("c1", "owner stays Alice")
	if p.claveAlmacen() != "alice/acme" || p.dueño() != "alice" {
		t.Fatal("private owner came from mutable Suite callbacks")
	}
	pending, err := work.Pending(t.Context())
	if err != nil || pending.Count != 1 || pending.Scope != scope {
		t.Fatal(pending, err)
	}
	count, err := journal.CountOwner("bob/elsewhere", espacioBorradores)
	if err != nil || count != 0 {
		t.Fatal("draft written as the next user", count, err)
	}
	if _, err := NuevaPantallaPrivada(t.Context(), work, trInterno(t), "es", privatePresenterData{scope: storage.Scope{Tenant: "acme", Subject: "bob", Product: "notes"}}, nil, nil); err == nil {
		t.Fatal("foreign data facade mounted")
	}
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPrivatePresenterRejectsUnadmittedCaptureWithoutReplacingSavedDraft(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	journal := almacenDePrueba(t)
	denied := false
	work, err := notas.NewLocalWork(t.Context(), scope, journal, func(context.Context) (func(), error) {
		if denied {
			return nil, errors.New("paused original witness")
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
	if err := p.guardarBorradorConError("c1", "accepted"); err != nil {
		t.Fatal(err)
	}
	denied = true
	if err := p.guardarBorradorConError("c1", "not admitted"); !errors.Is(err, notas.ErrLocalAdmission) {
		t.Fatal(err)
	}
	if text := p.borradorGuardado("c1"); text != "accepted" {
		t.Fatal("unadmitted capture replaced accepted snapshot", text)
	}
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateEditorRestoresLastAcceptedTextWhenOriginalLeaseIsDenied(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	journal := almacenDePrueba(t)
	work, err := notas.NewLocalWork(t.Context(), scope, journal, func(context.Context) (func(), error) { return nil, errors.New("original witness paused") })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	p, err := NuevaPantallaPrivada(t.Context(), work, trInterno(t), "es", privatePresenterData{scope: scope}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.activo = DeCuaderno(api.Notebook{ID: "c1", OwnerSub: "alice", Visibility: "personal"})
	p.notas = DeNotas([]api.Note{{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>accepted</p>", UpdatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}})
	n, _ := p.nota("n1")
	p.abrirEditor(n)
	teclear(p.edicion.editor.zona, " rejected")
	if html := p.edicion.editor.HTML(); html != "<p>accepted</p>" || p.edicion.html != html {
		t.Fatal("editor silently accepted text after lease denial", html, p.edicion.html)
	}
	if pending, err := work.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("unadmitted editor text became pending work", pending, err)
	}
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateNotebookMigrationDrainsItsCurrentWorkPhase(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(canceled) }))
	t.Cleanup(server.Close)
	journal := almacenDePrueba(t)
	work, err := notas.NewLocalWork(t.Context(), storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}, journal, func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	p := &Pantalla{svc: api.NewNotes(server.URL, nil, server.Client()), Almacen: journal, Plazo: 30 * time.Second, lifetime: vidaPantalla{work: work, dispatch: func(fn func()) { fn() }, dispatchWait: func(fn func()) { fn() }}, activo: Cuaderno{ID: "c1", Cifrado: true}, clave: make([]byte, 32), notas: DeNotas([]api.Note{{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>legacy inner encryption</p>"}})}
	t.Cleanup(p.Cerrar)
	p.migrarContenido()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("inner notebook migration did not issue its source read")
	}
	deadline, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := work.Drain(deadline); err != nil {
		t.Fatal("inner notebook migration did not inherit its work phase", err)
	}
	select {
	case <-canceled:
	case <-deadline.Done():
		t.Fatal("drain returned before notebook source request canceled")
	}
}

func TestPrivateEditorCannotInstallRemoteAIFromLegacyCallback(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	journal := almacenDePrueba(t)
	work, err := notas.NewLocalWork(t.Context(), scope, journal, func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(work.Close)
	p, err := NuevaPantallaPrivada(t.Context(), work, trInterno(t), "es", privatePresenterData{scope: scope}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	p.Asistir = func(context.Context, api.PeticionIA) (string, error) { calls.Add(1); return "remote", nil }
	p.activo = DeCuaderno(api.Notebook{ID: "c1", OwnerSub: "alice", Visibility: "personal"})
	p.notas = DeNotas([]api.Note{{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>private text</p>"}})
	n, _ := p.nota("n1")
	p.abrirEditor(n)
	if p.edicion.editor.Asistir != nil {
		t.Fatal("private editor installed an unscoped remote AI callback")
	}
	p.edicion.editor.PedirIA(func(text string) api.PeticionIA { return api.PeticionIA{Contexto: text} })
	if calls.Load() != 0 {
		t.Fatal("private content reached remote AI")
	}
	p.Cerrar()
	if err := p.Join(t.Context()); err != nil {
		t.Fatal(err)
	}
}
