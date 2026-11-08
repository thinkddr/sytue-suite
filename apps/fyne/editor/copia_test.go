// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Hacer una copia» (`hacerCopia` de la web): guarda ANTES, pide la copia con el título
// traducido y abre la que vuelve. Un guardado fallido no deja copiar —saldría sin lo
// último escrito— y un fallo se avisa con `copy.error`.
//
// VISTO EN ROJO (2026-11-05): con `antes` llamado DESPUÉS de crear la copia, el servidor
// recibía la petición sin el guardado y el test lo decía.
func TestHacerUnaCopiaGuardaAntesYAbreLaCopia(t *testing.T) {
	var guardado atomic.Bool
	var cuerpo string
	var peticiones atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peticiones.Add(1)
		if !guardado.Load() {
			t.Error("se pidió la copia antes de guardar")
		}
		b, _ := io.ReadAll(r.Body)
		cuerpo = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"document":{"id":"c1","title":"Copia de Acta","kind":"document"},"role":"owner"}`)
	}))
	defer srv.Close()
	var avisos []string
	s := Servicio{
		Svc: api.NewDocs(srv.URL, nil, srv.Client()), Lanzar: func(f func()) { f() },
		T: func(k string, v i18n.Vars) string {
			if t, ok := v["title"].(string); ok {
				return k + ":" + t
			}
			return k
		},
		Avisar: func(_ ui.TonoAviso, titulo string) { avisos = append(avisos, titulo) },
	}
	var abierta api.Document
	s.HacerCopia(api.Document{ID: "d1", Kind: api.DocText, Title: "Acta"},
		func() error { guardado.Store(true); return nil }, func(d api.Document) { abierta = d }, nil)
	if abierta.ID != "c1" || len(avisos) != 0 {
		t.Fatalf("la copia abierta: %+v, avisos %v", abierta, avisos)
	}
	if cuerpo != `{"kind":"document","title":"copy.title:Acta","copy_of":"d1"}` {
		t.Errorf("cuerpo: %s", cuerpo)
	}

	abierta = api.Document{}
	s.HacerCopia(api.Document{ID: "d1", Kind: api.DocText, Title: "Acta"},
		func() error { return errors.New("sin guardar") }, func(d api.Document) { abierta = d }, nil)
	if peticiones.Load() != 1 || abierta.ID != "" || len(avisos) != 1 || avisos[0] != "copy.error" {
		t.Errorf("con el guardado fallido: %d peticiones, abierta %+v, avisos %v", peticiones.Load(), abierta, avisos)
	}
}

// La entrada del menú Archivo y el botón de la tesela llaman al producto; sin gancho, la
// entrada no sale, y sin red está inerte.
func TestHacerUnaCopiaEnArchivoYEnLaTesela(t *testing.T) {
	ctx := &Contexto{Tipo: Presentacion}
	for _, tx := range textos(NuevoBotonArchivo(ctx).Menu()) {
		if tx == "copy.action" {
			t.Fatal("sin gancho, «Hacer una copia» no sale")
		}
	}
	copias := 0
	ctx.HacerCopia = func(listo func()) { copias++; listo() }
	m := NuevoBotonArchivo(ctx).Menu()
	var entrada *ui.ElementoMenu
	for _, e := range m.Elementos {
		if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == "copy.action" {
			entrada = it
		}
	}
	if entrada == nil {
		t.Fatalf("el menú Archivo no tiene «Hacer una copia»: %v", textos(m))
	}
	entrada.AlPulsar()
	if copias != 1 {
		t.Error("«Hacer una copia» no llama al producto")
	}
	ctx.SinRed = true
	for _, e := range NuevoBotonArchivo(ctx).Menu().Elementos {
		if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == "copy.action" && !it.Inerte {
			t.Error("sin red, «Hacer una copia» tiene que salir inerte")
		}
	}

	var copiado api.Document
	d := api.Document{ID: "d1", Kind: api.DocSheet, Title: "Cuentas"}
	s := NuevaTeselaDoc(d, nil, "es", AccionesTesela{Copiar: func(x api.Document) { copiado = x }})
	if len(s.acciones) != 3 || s.acciones[1].Rotulo != "tile.copy.label" {
		t.Fatalf("la tesela: renombrar · copiar · eliminar; tiene %d", len(s.acciones))
	}
	s.acciones[1].AlPulsar()
	if copiado.ID != "d1" {
		t.Errorf("el botón de copiar no llama a Copiar: %+v", copiado)
	}
}

// servidorDeCopias cuenta las copias pedidas y devuelve la que creó.
func servidorDeCopias(t *testing.T, cuerpos *[]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		b, _ := io.ReadAll(r.Body)
		if cuerpos != nil {
			*cuerpos = append(*cuerpos, string(b))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"document":{"id":"c1","title":"Acta","kind":"document"},"role":"owner"}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

// Un doble clic en el botón de copia de la tesela pide UNA copia (`copiando` de
// DocList.tsx): la tesela no es un menú que se cierra. Acabada, se puede volver a copiar.
//
// VISTO EN ROJO (2026-11-06): sin la guarda, dos pulsaciones eran dos POST /documents y
// se abrían dos «Copia de …».
func TestLaTeselaNoHaceDosCopiasConUnDobleClic(t *testing.T) {
	srv, n := servidorDeCopias(t, nil)
	var enCola []func()
	p := NuevaPortada(Documento, nil, "es", api.NewDocs(srv.URL, nil, srv.Client()))
	p.Lanzar = func(f func()) { enCola = append(enCola, f) }
	abiertas := 0
	p.AlAbrir = func(api.Document) { abiertas++ }
	d := api.Document{ID: "d1", Kind: api.DocText, Title: "Acta"}
	p.Copiar(d)
	p.Copiar(d)
	for _, f := range enCola {
		f()
	}
	fyne.DoAndWait(func() {})
	if n.Load() != 1 || abiertas != 1 {
		t.Fatalf("un doble clic: %d copias pedidas, %d abiertas", n.Load(), abiertas)
	}
	enCola = nil
	p.Copiar(d)
	for _, f := range enCola {
		f()
	}
	fyne.DoAndWait(func() {})
	if n.Load() != 2 {
		t.Errorf("acabada la primera, se puede volver a copiar: %d", n.Load())
	}
}

// En el menú Archivo, mientras la copia vuelve la entrada sale inerte y una segunda
// pulsación no pide otra (`copiando` de Ribbon.tsx).
//
// VISTO EN ROJO (2026-11-06): sin `Copiando`, la entrada seguía encendida y llamaba
// otra vez al producto.
func TestArchivoNoHaceDosCopiasALaVez(t *testing.T) {
	ctx := &Contexto{Tipo: Documento}
	var pendientes []func()
	ctx.HacerCopia = func(listo func()) { pendientes = append(pendientes, listo) }
	entrada := func() *ui.ElementoMenu {
		for _, e := range NuevoBotonArchivo(ctx).Menu().Elementos {
			if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == "copy.action" {
				return it
			}
		}
		t.Fatal("el menú Archivo no tiene «Hacer una copia»")
		return nil
	}
	primera := entrada()
	primera.AlPulsar()
	primera.AlPulsar()
	if len(pendientes) != 1 {
		t.Fatalf("dos pulsaciones con la copia en curso: %d copias", len(pendientes))
	}
	if !entrada().Inerte {
		t.Error("con la copia en curso, la entrada sale inerte")
	}
	pendientes[0]()
	if entrada().Inerte {
		t.Error("acabada la copia, la entrada vuelve a estar encendida")
	}
}

// Sin traductor, el título de la copia es el del original y no un «Copia de …» en
// castellano escrito en el código: la clave, como en `avisar`.
func TestLaCopiaSinTraductorNoLlevaUnLiteral(t *testing.T) {
	var cuerpos []string
	srv, _ := servidorDeCopias(t, &cuerpos)
	s := Servicio{Svc: api.NewDocs(srv.URL, nil, srv.Client()), Lanzar: func(f func()) { f() }}
	s.HacerCopia(api.Document{ID: "d1", Kind: api.DocText, Title: "Acta"}, nil, nil, nil)
	fyne.DoAndWait(func() {})
	if len(cuerpos) != 1 || cuerpos[0] != `{"kind":"document","title":"Acta","copy_of":"d1"}` {
		t.Errorf("cuerpo: %v", cuerpos)
	}
}
