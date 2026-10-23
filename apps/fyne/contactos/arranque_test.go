// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package contactos

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestPrimerContenidoNoEsperaAlDirectorio(t *testing.T) {
	const delay = 350 * time.Millisecond
	started := time.Now()
	var first atomic.Int64
	var directoryCalls atomic.Int32
	var queue []func()
	contacts := make([]api.Contact, 601)
	for i := range contacts {
		contacts[i] = persona(fmt.Sprint(i), fmt.Sprintf("Persona %04d", i))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v any = map[string]any{}
		switch r.URL.Path {
		case "/api/v1/directory":
			directoryCalls.Add(1)
			time.Sleep(delay)
			v = map[string]any{"items": []api.Miembro{}}
		case "/api/v1/address-books":
			v = map[string]any{"address_books": []api.AddressBook{{ID: "l1", Name: "personal", IsDefault: true}, {ID: "l2", Name: "organizacion"}}}
		case "/api/v1/address-books/l1/contacts/page":
			first.CompareAndSwap(0, time.Since(started).Nanoseconds())
			start := 0
			_, _ = fmt.Sscan(r.URL.Query().Get("cursor"), &start)
			page := api.ContactPage{Contacts: contacts[start:min(start+100, len(contacts))]}
			if start+100 < len(contacts) {
				page.NextCursor = fmt.Sprint(start + 100)
			}
			v = page
		case "/api/v1/address-books/l2/contacts/page":
			v = api.ContactPage{}
		}
		_ = json.NewEncoder(w).Encode(v)
	}))
	defer srv.Close()
	a := NuevaApp(traductor(t), api.NewContacts(srv.URL, nil, nil))
	defer a.Cerrar()
	a.Directorio = api.NewDirectorio(srv.URL, nil, nil)
	a.enFondo = func(f func()) { queue = append(queue, f) }
	a.Cargar()
	if len(a.Pantalla.Contactos) != 601 || directoryCalls.Load() != 0 {
		t.Fatalf("visibles=%d directorio=%d", len(a.Pantalla.Contactos), directoryCalls.Load())
	}
	t.Logf("directorio=%s primerGETpersonal=%s contenido601=%s", delay, time.Duration(first.Load()), time.Since(started))
	// The optional result must preserve the selected book/contact and search.
	selected := a.todos[3]
	a.Pantalla.Seleccionado = &selected
	a.Pantalla.Busqueda = "Persona"
	for _, f := range queue {
		f()
	}
	if directoryCalls.Load() != 1 || a.Pantalla.Activa.ID != "l1" || a.Pantalla.Seleccionado == nil || a.Pantalla.Seleccionado.ID != selected.ID || a.Pantalla.Busqueda != "Persona" {
		t.Fatal("el resultado opcional robó la selección")
	}
}

func TestCerrarOVaciarCancelaDirectorioYDescartaResultados(t *testing.T) {
	for _, closeWindow := range []bool{false, true} {
		t.Run(fmt.Sprint(closeWindow), func(t *testing.T) {
			entered := make(chan struct{})
			canceled := make(chan struct{})
			var posts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
				}
				if strings.HasSuffix(r.URL.Path, "/directory") {
					close(entered)
					<-r.Context().Done()
					close(canceled)
					return
				}
				_, _ = w.Write([]byte(`{"address_books":[],"contacts":[]}`))
			}))
			defer srv.Close()
			a := NuevaApp(traductor(t), api.NewContacts(srv.URL, nil, nil))
			a.Directorio = api.NewDirectorio(srv.URL, nil, nil)
			scope := a.capturar()
			done := make(chan struct{})
			go func() { defer close(done); a.sincronizarOrganizacionEn(scope) }()
			<-entered
			if closeWindow {
				a.Cerrar()
			} else {
				a.Vaciar()
			}
			select {
			case <-canceled:
			case <-time.After(2 * time.Second):
				t.Fatal("directorio no cancelado")
			}
			<-done
			if a.vigente(scope) || posts.Load() != 0 || len(a.Pantalla.Libretas) != 0 {
				t.Fatal("reaparecieron datos de otra identidad")
			}
		})
	}
}

func TestCargarMismaIdentidadConservaSeleccion(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("a", "Ana")}})
	a.Cargar()
	selected := a.todos[0]
	a.Pantalla.Seleccionado = &selected
	a.Pantalla.Busqueda = "Ana"
	scope := a.capturar()
	a.Cargar()
	if !a.vigente(scope) || a.Pantalla.Seleccionado == nil || a.Pantalla.Seleccionado.ID != "a" || a.Pantalla.Busqueda != "Ana" {
		t.Fatal("refrescar perdió el estado de la misma identidad")
	}
}

// A queued old-account click may run after the new account has already loaded.
// It must not navigate its book or even expose its old book name.
func TestCambioIdentidadDescartaLibretaEncolada(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("a", "Ana")}})
	a.Cargar()
	var pending []func()
	a.enFondo = func(f func()) { pending = append(pending, f) }
	a.Pantalla.AlElegirLibreta(api.AddressBook{ID: "l2", Name: "organizacion", DisplayName: "Nombre anterior"})
	old := pending[0]
	a.Vaciar()
	a.Cargar()
	old()
	if a.Pantalla.Activa == nil || a.Pantalla.Activa.ID != "l1" || len(a.Pantalla.Contactos) != 1 || a.Pantalla.Contactos[0].ID != "a" {
		t.Fatal("una acción de la identidad anterior cambió la libreta nueva")
	}
}
