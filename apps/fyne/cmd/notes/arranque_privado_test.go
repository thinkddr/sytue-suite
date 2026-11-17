// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// La PRUEBA DE ARRANQUE REAL del presentador privado: monta la app por el mismo
// camino que `main` —`montarNotepads` sobre el dueño registrado— y comprueba que la
// primera carga sale de la fachada privada del dispositivo, no de un cliente HTTP
// legible, y que el ciclo cierra sin perder trabajo. Corre con el pintor de software.

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	os.Exit(m.Run())
}

func traductorDeArranque(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Notes())
	if err != nil {
		t.Fatalf("no se pudo cargar el catálogo: %v", err)
	}
	return tr
}

// arranqueBackend es el dueño de los datos del dispositivo verificado: cuenta lo que
// el arranque le pide. Los métodos que la primera carga no toca quedan en el cero del
// interfaz embebido: si el presentador los llamara, el test fallaría ruidosamente.
type arranqueBackend struct {
	lifecycleNotesRepository
	cuadernos atomic.Int32
}

func (b *arranqueBackend) ListNotebooks(context.Context) ([]api.Notebook, error) {
	b.cuadernos.Add(1)
	return []api.Notebook{{ID: "c1", OwnerSub: b.scope.Subject, Visibility: "personal"}}, nil
}

func (b *arranqueBackend) ListTags(context.Context) ([]api.Tag, error) { return nil, nil }

func (b *arranqueBackend) ListNotes(context.Context, string) ([]api.Note, error) { return nil, nil }

func (b *arranqueBackend) ListReminders(context.Context) ([]api.NoteReminder, error) { return nil, nil }

func arranqueNotepads(t *testing.T) (*notepadsRepository, *arranqueBackend) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	local, err := notas.NewLocalWork(t.Context(), scope, journal, func(context.Context) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	backend := &arranqueBackend{}
	backend.scope = scope
	owner := &notepadsRepository{notesRepository: backend, local: local, life: t.Context()}
	t.Cleanup(owner.Close)
	return owner, backend
}

// TestMontarNotepadsArrancaSobreElDuenoPrivado: el arranque carga por la fachada
// privada y el cierre drena sin dejar pendientes. La bomba de `fyne.DoAndWait` sirve
// de punto de sincronización del pintor de software, como una captura de píxeles.
func TestMontarNotepadsArrancaSobreElDuenoPrivado(t *testing.T) {
	owner, backend := arranqueNotepads(t)
	p, err := montarNotepads(t.Context(), owner, traductorDeArranque(t), "es", nil, nil, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	fyne.DoAndWait(p.Arrancar)
	deadline := time.Now().Add(5 * time.Second)
	for backend.cuadernos.Load() == 0 && time.Now().Before(deadline) {
		fyne.DoAndWait(func() {})
	}
	if backend.cuadernos.Load() == 0 {
		t.Fatal("el arranque no pidió los cuadernos al dueño privado")
	}
	var linked bool
	fyne.DoAndWait(func() { linked = p.AtenderEnlace(&url.URL{Path: "/notebooks/c1"}) })
	if !linked {
		t.Fatal("un enlace de Notepads no se atendió en el presentador montado")
	}
	fyne.DoAndWait(p.Cerrar)
	joinNotepadsWithUILoop(t, p)
	if pending, err := owner.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("el arranque dejó trabajo pendiente en el dueño", pending, err)
	}
}

// TestMontarNotepadsRechazaArrancarSinDueno: sin repositorio registrado no hay
// arranque, y el fallo es el de recursos privados —nunca una pantalla vacía legible.
func TestMontarNotepadsRechazaArrancarSinDueno(t *testing.T) {
	if _, err := montarNotepads(t.Context(), nil, traductorDeArranque(t), "es", nil, nil, nil); err == nil {
		t.Fatal("se montó un presentador sin dueño privado")
	}
}

type ventanaDeCierre struct {
	interceptar func()
	cerrada     bool
}

func (w *ventanaDeCierre) SetCloseIntercept(fn func()) { w.interceptar = fn }
func (w *ventanaDeCierre) Close()                      { w.cerrada = true }

func TestIntercepcionDeCierrePermiteReintentarBorradorPendiente(t *testing.T) {
	w := &ventanaDeCierre{}
	durable := false
	intentos := 0
	interceptarCierre(w, func() bool { intentos++; return durable })
	w.interceptar()
	if w.cerrada || w.interceptar == nil || intentos != 1 {
		t.Fatal("la ventana cerró o perdió la opción de reintentar")
	}
	durable = true
	w.interceptar()
	if !w.cerrada || w.interceptar != nil || intentos != 2 {
		t.Fatal("el cierre no continuó tras conservar el borrador")
	}
}

// The real application keeps its UI event loop running while a worker joins the
// presenter. The software driver must do the same: a blocking Join on this test
// goroutine would strand an already queued DoAndWait callback after navigation.
func joinNotepadsWithUILoop(t *testing.T, p *notes.Pantalla) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	joined := make(chan error, 1)
	go func() { joined <- p.Join(ctx) }()
	for {
		select {
		case err := <-joined:
			if err != nil {
				t.Fatal(err)
			}
			return
		case <-ctx.Done():
			t.Fatal("private presenter did not join while the UI loop was running")
		default:
			fyne.DoAndWait(func() {})
		}
	}
}
