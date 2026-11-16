// SPDX-License-Identifier: Apache-2.0
package contactos

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestCerrarCancelaYEsperaTrabajoAdmitido(t *testing.T) {
	a := NuevaApp(traductor(t), nil)
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	a.lanzar(func() {
		ctx := a.capturar().ctx
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
	})
	<-started
	a.Cerrar()
	<-canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Esperar(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Esperar con trabajo pendiente: %v", err)
	}
	ran := make(chan struct{}, 1)
	a.lanzar(func() { ran <- struct{}{} })
	close(release)
	if err := a.Esperar(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
		t.Fatal("se admitió trabajo después de cerrar")
	default:
	}
}

func TestSelectorDeIdentidadAnteriorNoIniciaImportacion(t *testing.T) {
	a := NuevaApp(traductor(t), api.NewContacts("https://example.invalid", nil, nil))
	var selected func([]byte)
	a.ElegirFichero = func(_ []string, done func([]byte)) { selected = done }
	called := make(chan struct{}, 1)
	a.elegirEImportar([]string{".csv"}, func(cargaContacts, []byte) { called <- struct{}{} })
	if selected == nil {
		t.Fatal("no se abrió el selector")
	}
	a.Vaciar()
	selected([]byte("old identity"))
	a.Cerrar()
	if err := a.Esperar(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("se inició la importación de la identidad anterior")
	default:
	}
}

// A task may pass the scheduler's guard before the presenter is cleared. The
// operation must still use the original, now-cancelled identity afterwards.
func TestVaciarDuranteTrabajoNoAdoptaIdentidadNueva(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	a := NuevaApp(traductor(t), api.NewContacts(srv.URL, nil, nil))
	started := make(chan struct{})
	resume := make(chan struct{})
	done := make(chan struct{})
	a.programar(func(scope cargaContacts) {
		close(started)
		<-resume
		a.guardarContacto(scope, api.AddressBook{ID: "old-book"}, nil, api.ContactInput{FormattedName: "private"}, "", false, func(error) {})
		close(done)
	})
	<-started
	a.Vaciar()
	newScope := a.capturar()
	if !a.vigente(newScope) {
		t.Fatal("la nueva identidad no se pudo preparar")
	}
	close(resume)
	<-done
	a.Cerrar()
	if err := a.Esperar(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("la operación anterior usó la identidad nueva")
	}
}

// Returning from a selector before clearing but running its queued import after
// clearing must retain the selector's original identity, not capture a new one.
func TestImportacionEncoladaConservaIdentidadDelSelector(t *testing.T) {
	a := NuevaApp(traductor(t), api.NewContacts("https://example.invalid", nil, nil))
	var selected func([]byte)
	var pending func()
	a.ElegirFichero = func(_ []string, done func([]byte)) { selected = done }
	a.enFondo = func(f func()) { pending = f }
	called := false
	a.elegirEImportar([]string{".csv"}, func(cargaContacts, []byte) { called = true })
	selected([]byte("old identity"))
	if pending == nil {
		t.Fatal("no se encoló la importación")
	}
	a.Vaciar()
	_ = a.capturar()
	pending()
	if called {
		t.Fatal("la importación de la identidad anterior se ejecutó")
	}
	a.Cerrar()
}

// Old work can no longer clear its UI flags after cancellation. Reusing the
// presenter must reset them without waiting for that canceled callback.
func TestVaciarLiberaEstadoImportacionYLote(t *testing.T) {
	a := NuevaApp(traductor(t), nil)
	a.Pantalla.ImportOcupado = true
	a.Pantalla.Importando = "Importando 1 de 2"
	a.Pantalla.LoteOcupado = true
	a.Pantalla.CargandoLibretas, a.Pantalla.ErrorLibretas = true, true
	a.Pantalla.CargandoEtiquetas, a.Pantalla.ErrorEtiquetas = true, true
	a.Pantalla.DetalleAbierto = true
	a.Vaciar()
	if a.Pantalla.ImportOcupado || a.Pantalla.Importando != "" || a.Pantalla.LoteOcupado ||
		a.Pantalla.CargandoLibretas || a.Pantalla.ErrorLibretas || a.Pantalla.CargandoEtiquetas || a.Pantalla.ErrorEtiquetas || a.Pantalla.DetalleAbierto {
		t.Fatal("los trabajos de la identidad anterior bloquearon la interfaz nueva")
	}
}
