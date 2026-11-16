// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// controlDe es el control del campo `clave` de la ficha.
func controlDe(t *testing.T, f *Ficha, clave string) *Control {
	t.Helper()
	for _, k := range f.controles {
		if k.Campo.Clave == clave {
			return k
		}
	}
	t.Fatalf("la ficha no tiene el campo %q", clave)
	return nil
}

// TestLaFichaPreguntaAntesDePerderLosCambios: cerrar con algo escrito sin guardar
// pregunta; sin cambios cierra como siempre, Escape sobre la pregunta es «Seguir
// editando» (y no deja capas), y «Descartar» cierra.
func TestLaFichaPreguntaAntesDePerderLosCambios(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.Atajos(w.Canvas(), nil)
	tecla := func(n fyne.KeyName) { w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: n}) }
	registros := p.lista.Pagina.Registros

	// Sin cambios: el botón Cerrar cierra en el acto.
	p.AbrirFicha(registros[0])
	if p.ficha == nil || p.ficha.Cambiada() {
		t.Fatal("la ficha recién abierta no puede tener cambios")
	}
	p.ficha.AlCerrar()
	if p.ficha != nil || dialogoArriba(w) != nil {
		t.Fatal("sin cambios, cerrar no pregunta")
	}

	p.AbrirFicha(registros[0])
	f := p.ficha
	externo := controlDe(t, f, "externo_id").entrada
	test.Type(externo, "X-42")
	if !f.Cambiada() {
		t.Fatal("teclear en un campo es un cambio")
	}

	// «Atrás» pregunta y no cierra.
	tecla(mobile.KeyBack)
	d := dialogoArriba(w)
	if d == nil || p.ficha != f {
		t.Fatal("con cambios, cerrar debe preguntar y dejar la ficha abierta")
	}
	if !contiene(textosDe(d), p.t("form.discardTitle")) || !contiene(textosDe(d.Pie), p.t("form.keepEditing")) {
		t.Errorf("la pregunta no dice lo que pasa: %v", textosDe(d))
	}

	// Escape sobre la pregunta es «Seguir editando»: sin capas, con lo escrito.
	tecla(fyne.KeyEscape)
	if dialogoArriba(w) != nil || w.Canvas().Overlays().Top() != nil {
		t.Fatal("Escape debe cerrar la pregunta sin dejar capas")
	}
	if p.ficha != f || f.Valores["externo_id"] != "X-42" {
		t.Fatalf("«Seguir editando» debe conservar la ficha y su valor; hay %q", f.Valores["externo_id"])
	}

	// Abrir otro registro también pregunta; «Seguir editando» se queda en esta.
	p.AbrirFicha(registros[1])
	d = dialogoArriba(w)
	if d == nil || p.ficha != f {
		t.Fatal("abrir otro registro con cambios debe preguntar antes")
	}
	pulsarDelPie(t, d, p.t("form.keepEditing"))
	if p.ficha != f || dialogoArriba(w) != nil {
		t.Fatal("«Seguir editando» no debe abrir el otro registro")
	}

	// Cambiar de vista pregunta, y «Descartar» se va.
	p.IrA(VistaTareas)
	d = dialogoArriba(w)
	if d == nil || p.Vista == VistaTareas {
		t.Fatal("cambiar de vista con cambios debe preguntar antes")
	}
	pulsarDelPie(t, d, p.t("form.discard"))
	if p.ficha != nil || p.Vista != VistaTareas || w.Canvas().Overlays().Top() != nil {
		t.Fatal("«Descartar» debe cerrar la ficha e ir a donde se pidió")
	}
}

// TestGuardarNoPreguntaPorLosCambios: lo guardado ya no se pierde al cerrar.
func TestGuardarNoPreguntaPorLosCambios(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.Atajos(w.Canvas(), nil)
	p.AbrirFicha(p.lista.Pagina.Registros[0])
	test.Type(controlDe(t, p.ficha, "externo_id").entrada, "X-42")
	p.escrito(p.ficha.Modelo)
	if p.ficha != nil || dialogoArriba(w) != nil {
		t.Fatal("guardar con éxito cierra la ficha sin preguntar")
	}
}

// TestElMenuYLaPaletaPreguntanAntesDeDejarLaFicha: los tableros del menú y la paleta
// también sacan de la ficha. La paleta se cierra antes de preguntar, como en la web.
func TestElMenuYLaPaletaPreguntanAntesDeDejarLaFicha(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.Atajos(w.Canvas(), nil)
	p.Tableros = []api.ERPTablero{{ID: "t1", Titulo: "Ventas"}}
	modelo := p.lista.Modelo
	p.AbrirFicha(p.lista.Pagina.Registros[0])
	f := p.ficha
	test.Type(controlDe(t, f, "externo_id").entrada, "X-42")

	p.destinosTableros()[0].AlPulsar()
	d := dialogoArriba(w)
	if d == nil || p.Vista == VistaTablero || p.ficha != f {
		t.Fatal("ir a un tablero del menú con cambios debe preguntar antes")
	}
	pulsarDelPie(t, d, p.t("form.keepEditing"))
	if p.ficha != f || p.Vista == VistaTablero {
		t.Fatal("«Seguir editando» no debe ir al tablero")
	}

	p.AbrirPaleta()
	p.paleta.AlElegir(modelo)
	d = dialogoArriba(w)
	if d == nil || p.abierta || p.ficha != f {
		t.Fatal("elegir en la paleta con cambios debe cerrarla y preguntar antes")
	}
	pulsarDelPie(t, d, p.t("form.discard"))
	if p.ficha != nil || w.Canvas().Overlays().Top() != nil {
		t.Fatal("«Descartar» debe cerrar la ficha sin dejar capas")
	}
}

// TestDuplicarDeLaListaPreguntaAntesDePedirLaCopia: «Duplicar» en una fila hace la copia
// en el servidor. Con la ficha abierta y cambios sin guardar, la pregunta va ANTES:
// «Seguir editando» no puede dejar en el servidor un borrador que nadie ha visto.
func TestDuplicarDeLaListaPreguntaAntesDePedirLaCopia(t *testing.T) {
	var copias atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/duplicar") {
			copias.Add(1)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.svc = api.NewERP(srv.URL, nil, nil, "es")
	p.fondo = func(f func()) { f() }
	p.Atajos(w.Canvas(), nil)
	registros := p.lista.Pagina.Registros
	p.AbrirFicha(registros[0])
	f := p.ficha
	test.Type(controlDe(t, f, "externo_id").entrada, "X-42")

	p.duplicarDeLista(registros[1])
	d := dialogoArriba(w)
	if d == nil || copias.Load() != 0 {
		t.Fatalf("con cambios, duplicar debe preguntar antes de pedir la copia (%d pedidas)", copias.Load())
	}
	pulsarDelPie(t, d, p.t("form.keepEditing"))
	if copias.Load() != 0 || p.ficha != f || f.Valores["externo_id"] != "X-42" {
		t.Fatal("«Seguir editando» no debe pedir la copia ni dejar la ficha")
	}
}
