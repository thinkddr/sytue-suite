// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// selectorDe es el selector con ese nombre accesible.
func selectorDe(w fyne.Window, rotulo string) *ui.Selector {
	var hallado *ui.Selector
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if s, ok := o.(*ui.Selector); ok && s.Rotulo == rotulo {
			hallado = s
		}
	})
	return hallado
}

// botonIcono es el botón de icono con ese nombre accesible.
func botonIcono(w fyne.Window, rotulo string) *ui.BotonIcono {
	var hallado *ui.BotonIcono
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.BotonIcono); ok && b.Rotulo == rotulo {
			hallado = b
		}
	})
	return hallado
}

// TestBuscarYOrdenarSinGuardarUnaVista: la búsqueda y el orden de un momento viajan al
// servicio (`q`, `sort`) y vuelven a la primera página: nunca se filtra ni se ordena
// solo la ventana cargada. No se ofrece ordenar por enlaces ni adjuntos, y cambiar de
// tabla los olvida.
func TestBuscarYOrdenarSinGuardarUnaVista(t *testing.T) {
	f := nuevoFalso(450)
	p, w := abrir(t, f)
	b := p.abierta
	if b.busqueda == nil || selectorDe(w, "Ordenar por") == nil {
		t.Fatalf("la rejilla no tiene buscador ni «Ordenar por»:\n%s", textos(w))
	}
	if got := selectorDe(w, "Ordenar por").Opciones; slices.Contains(got, "Empresa") || got[0] != "Orden de la vista" || !slices.Contains(got, "Nombre") {
		t.Errorf("opciones de orden: %q", got)
	}

	// Intro busca sin esperar; los «Persona 7…» están repartidos por toda la tabla.
	fyne.DoAndWait(func() {
		b.busqueda.PonerTexto("persona 7")
		b.busqueda.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	})
	if n := len(b.rejilla.Registros); n != 11 {
		t.Fatalf("la búsqueda devuelve %d registros, quería 11", n)
	}

	fyne.DoAndWait(func() { selectorDe(w, "Ordenar por").AlElegir(1) })
	asc := botonIcono(w, "Ascendente")
	if asc == nil {
		t.Fatalf("con un campo elegido falta el botón de la dirección:\n%s", textos(w))
	}
	test.Tap(asc)
	if botonIcono(w, "Descendente") == nil || b.rejilla.Registros[0].ID != "r079" {
		t.Fatalf("el orden descendente no llega: primero %s", b.rejilla.Registros[0].ID)
	}
	capturar(t, w, "rejilla-buscar-ordenar")

	// Escribir sin Intro espera a que se deje de teclear.
	fyne.DoAndWait(func() { b.busqueda.PonerTexto("nadie") })
	if len(b.rejilla.Registros) != 11 {
		t.Fatal("la búsqueda no espera a que se deje de teclear")
	}
	// Vencida la espera, lo pendiente llega por el hilo de la interfaz.
	time.Sleep(esperaBusqueda + 100*time.Millisecond)
	fyne.DoAndWait(func() {})
	fyne.DoAndWait(func() {})
	if n := len(b.rejilla.Registros); n != 0 {
		t.Errorf("tras la espera siguen %d registros", n)
	}
	if got := textos(w); !strings.Contains(got, "Ningún registro coincide con la búsqueda") {
		t.Errorf("sin coincidencias no se dice:\n%s", got)
	}

	quiero := []string{
		"GET /tables/t1/records?limit=100",
		"GET /tables/t1/records?limit=100&q=persona+7",
		"GET /tables/t1/records?limit=100&q=persona+7&sort=nom%3Aasc",
		"GET /tables/t1/records?limit=100&q=persona+7&sort=nom%3Adesc",
		"GET /tables/t1/records?limit=100&q=nadie&sort=nom%3Adesc",
	}
	if got := f.peticiones("GET /tables/t1/records"); !slices.Equal(got, quiero) {
		t.Errorf("peticiones de registros:\n%q\nquiero:\n%q", got, quiero)
	}

	fyne.DoAndWait(func() { p.elegirTabla("t2"); p.elegirTabla("t1") })
	if b.lectura.hay() || b.busqueda.Texto() != "" || len(b.rejilla.Registros) != 100 {
		t.Errorf("otra tabla no olvida la búsqueda: %+v, %d registros", b.lectura, len(b.rejilla.Registros))
	}
}

// Lo buscado en la rejilla sigue al pasar a un tablero: allí también está la barra para
// verlo y quitarlo, en vez de filtrar las tarjetas sin decirlo.
func TestElTableroLlevaLaBarraDeLectura(t *testing.T) {
	f := nuevoFalso(4)
	f.vistas["t1"] = []m{{"id": "vk", "table_id": "t1", "name": "Por nivel", "kind": "kanban", "config": m{"group_by": "niv"}}}
	p, w := abrir(t, f)
	b := p.abierta
	fyne.DoAndWait(func() {
		b.busqueda.PonerTexto("persona 1")
		b.busqueda.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	})
	elegir(t, w, "Vistas", "Por nivel · Tablero")
	if _, hay := b.campoDelTablero(); !hay {
		t.Fatal("la vista no se pinta como tablero")
	}
	if !f.vio("GET /tables/t1/records?limit=100&q=persona+1&view=vk") {
		t.Fatalf("el tablero no conserva la búsqueda: %q", f.peticiones("GET /tables/t1/records"))
	}
	enCanvas := false
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if o == b.busqueda {
			enCanvas = true
		}
	})
	if !enCanvas || selectorDe(w, "Ordenar por") == nil {
		t.Fatalf("el tablero filtra sin enseñar la barra:\n%s", textos(w))
	}
}
