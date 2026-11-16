// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"context"
	"errors"
	"image/color"
	"net/http"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type fotosPrivadasFixture struct {
	api.ContactsData
	read func(context.Context, string, string) ([]byte, error)
}

func (f fotosPrivadasFixture) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "tenant", Subject: "person", Product: "contacts"}
}

func (f fotosPrivadasFixture) PhotoByID(ctx context.Context, id, etag string) ([]byte, error) {
	return f.read(ctx, id, etag)
}

type noPhotoHTTP struct{ calls int }

func (h *noPhotoHTTP) RoundTrip(*http.Request) (*http.Response, error) {
	h.calls++
	return nil, errors.New("unexpected legacy HTTP")
}

func appFotosPrivadas(t *testing.T, fixture fotosPrivadasFixture) (*App, *noPhotoHTTP) {
	t.Helper()
	h := &noPhotoHTTP{}
	client, err := api.NewContacts("https://contacts.invalid", nil, &http.Client{Transport: h}).WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	a := NuevaApp(traductor(t), client)
	a.enUIFoto = func(f func()) { f() }
	w := test.NewWindow(a.Objeto())
	t.Cleanup(w.Close)
	a.Pantalla.Activa = &api.AddressBook{ID: "book"}
	return a, h
}

func TestFotosPrivadasUsanIDYRevisionAunqueUIDSeRepita(t *testing.T) {
	red := pngDe(t, color.NRGBA{R: 200, A: 255}, 2, 2)
	blue := pngDe(t, color.NRGBA{B: 200, A: 255}, 2, 2)
	var issued [][]byte
	fixture := fotosPrivadasFixture{read: func(_ context.Context, id, etag string) ([]byte, error) {
		if etag != "rev" {
			t.Fatal("revision changed")
		}
		var raw []byte
		switch id {
		case "a":
			raw = append([]byte(nil), red...)
		case "b":
			raw = append([]byte(nil), blue...)
		default:
			t.Fatal("unexpected contact ID")
		}
		issued = append(issued, raw)
		return raw, nil
	}}
	a, httpPort := appFotosPrivadas(t, fixture)
	first := api.Contact{ID: "a", AddressBookID: "book", UID: "same", ETag: "rev"}
	second := api.Contact{ID: "b", AddressBookID: "book", UID: "same", ETag: "rev"}
	a.todos = []api.Contact{first, second}
	a.pedirFoto(first, func(f func()) { f() })
	a.pedirFoto(second, func(f func()) { f() })
	for _, raw := range issued {
		for _, b := range raw {
			if b != 0 {
				t.Fatal("private encoded bytes survived the read")
			}
		}
	}
	firstImage, secondImage := a.fotoDe(first), a.fotoDe(second)
	if httpPort.calls != 0 || len(issued) != 2 || firstImage == nil || secondImage == nil {
		t.Fatal("private photos did not reach their contacts or reached HTTP")
	}
	r1, _, b1, _ := firstImage.At(0, 0).RGBA()
	r2, _, b2, _ := secondImage.At(0, 0).RGBA()
	if r1 <= b1 || b2 <= r2 {
		t.Fatal("private photos crossed contact IDs")
	}
}

func TestFotoPrivadaDescartaResultadoTrasCambioDeVistaOSesion(t *testing.T) {
	for _, mode := range []string{"revision", "book", "session"} {
		t.Run(mode, func(t *testing.T) {
			person := api.Contact{ID: "a", AddressBookID: "book", UID: "same", ETag: "r1"}
			raw := pngDe(t, color.NRGBA{R: 200, A: 255}, 2, 2)
			fixture := fotosPrivadasFixture{read: func(context.Context, string, string) ([]byte, error) { return raw, nil }}
			a, _ := appFotosPrivadas(t, fixture)
			a.todos = []api.Contact{person}
			a.Pantalla.Seleccionado = &person
			var apply func()
			a.enUIFoto = func(f func()) { apply = f }
			a.pedirFoto(person, func(f func()) { f() })
			if apply == nil {
				t.Fatal("the completed read did not schedule its UI handoff")
			}
			switch mode {
			case "revision":
				a.todos[0].ETag = "r2"
				a.Pantalla.Seleccionado = &a.todos[0]
			case "book":
				a.Pantalla.Activa = &api.AddressBook{ID: "other"}
			case "session":
				a.Vaciar()
			}
			apply()
			if a.fotoDe(person) != nil {
				t.Fatal("obsolete private photo was adopted")
			}
			for _, b := range raw {
				if b != 0 {
					t.Fatal("obsolete encoded bytes survived")
				}
			}
		})
	}
}

func TestFotosPrivadasLimitanLecturasYCancelanEspera(t *testing.T) {
	entered := make(chan string, 3)
	fixture := fotosPrivadasFixture{read: func(ctx context.Context, id, _ string) ([]byte, error) {
		entered <- id
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	a, _ := appFotosPrivadas(t, fixture)
	a.todos = []api.Contact{
		{ID: "a", AddressBookID: "book", ETag: "r1"},
		{ID: "b", AddressBookID: "book", ETag: "r1"},
		{ID: "c", AddressBookID: "book", ETag: "r1"},
	}
	a.pedirFoto(a.todos[0], a.lanzar)
	a.pedirFoto(a.todos[1], a.lanzar)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("two private reads did not start")
		}
	}
	a.pedirFoto(a.todos[2], a.lanzar)
	select {
	case <-entered:
		t.Fatal("more than two private reads started concurrently")
	default:
	}
	a.Cerrar()
	if err := a.Esperar(ctx); err != nil {
		t.Fatal("canceled reads or queued admission did not drain", err)
	}
	select {
	case <-entered:
		t.Fatal("waiting photo started after invalidation")
	default:
	}
}
