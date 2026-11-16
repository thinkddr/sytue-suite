// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// LA PORTADA NO SE QUEDA SIN SALIDA (y no salta al cargar).
//
// Si los recientes no llegaban, la portada decía «No se pudieron cargar tus PDF» y no
// había más que cerrar y volver a abrir; mientras cargaban, una rueda donde luego van
// las tarjetas. Es lo mismo que fija `Portada.test.tsx` en la web.

// driveConFallo es un Drive que falla la primera vez que se le pide la raíz y luego
// contesta con dos PDF en «PDF». Las peticiones de firma no existen (404): sin ellas
// no hay sección, que es lo que hace la portada.
type driveConFallo struct{ raices atomic.Int32 }

func (d *driveConFallo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/folders/root":
		if d.raices.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, `{"id":"raiz","name":"","is_root":true}`)
	case "/api/v1/folders/raiz/children":
		_, _ = io.WriteString(w, `{"folders":[{"id":"pdf","name":"PDF"}],"files":[]}`)
	case "/api/v1/folders/pdf/children":
		_, _ = io.WriteString(w, `{"folders":[],"files":[
			{"id":"a","folder_id":"pdf","name":"Contrato.pdf","updated_at":"2026-11-01T10:00:00Z"},
			{"id":"b","folder_id":"pdf","name":"Factura.pdf","updated_at":"2026-10-01T10:00:00Z"}]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// TestLaPortadaReintentaLosRecientesTrasUnFallo: un fallo enseña «Reintentar» junto a
// la frase, y pulsarlo trae la lista y pinta las tarjetas sin salir de la portada.
func TestLaPortadaReintentaLosRecientesTrasUnFallo(t *testing.T) {
	d := &driveConFallo{}
	srv := httptest.NewServer(d)
	defer srv.Close()
	pdf := api.NewPDF(srv.URL, srv.URL, nil, nil)

	p := NuevaPantalla(traductorES(t))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	listo := make(chan struct{}, 8)
	p.trasRed = func() { listo <- struct{}{} }
	p.MostrarPortada(pdf, "es")
	esperarRed(t, listo)

	m := p.Portada.marco
	if !p.Portada.Fallo {
		t.Fatal("el primer intento tenía que fallar")
	}
	fila, ok := m.cuerpo.(*fyne.Container)
	if !ok || len(fila.Objects) != 2 {
		t.Fatalf("tras el fallo el cuerpo es %T; tiene que ser la frase y «Reintentar»", m.cuerpo)
	}
	if r, ok := fila.Objects[0].(*ui.Rotulo); !ok || r.Texto != "No se pudieron cargar tus PDF." {
		t.Errorf("la frase del fallo es %#v", fila.Objects[0])
	}
	boton, ok := fila.Objects[1].(*ui.Boton)
	if !ok || boton.Texto != "Reintentar" || boton.Variante != ui.Secundario {
		t.Fatalf("al lado de la frase va %#v; tiene que ser el «Reintentar» secundario", fila.Objects[1])
	}

	fyne.DoAndWait(func() { fynetest.Tap(boton) })
	// Mientras llega la respuesta vuelven los esqueletos, no la frase.
	if p.Portada.Fallo || len(p.Portada.marco.esqueletos) != esqueletosRecientes {
		t.Errorf("al reintentar: fallo=%v y %d esqueletos", p.Portada.Fallo, len(p.Portada.marco.esqueletos))
	}
	esperarRed(t, listo)

	m = p.Portada.marco
	if p.Portada.Fallo || m.cuerpo != nil || len(m.esqueletos) != 0 {
		t.Errorf("tras reintentar: fallo=%v, cuerpo %T, %d esqueletos", p.Portada.Fallo, m.cuerpo, len(m.esqueletos))
	}
	if len(m.tarjetas) != 2 || m.tarjetas[0].nombre != "Contrato" || m.tarjetas[1].nombre != "Factura" {
		t.Errorf("tras reintentar hay %d tarjetas", len(m.tarjetas))
	}
	if n := d.raices.Load(); n != 2 {
		t.Errorf("se pidió la raíz %d veces; tienen que ser dos (el fallo y el reintento)", n)
	}
}

// TestLaPortadaCargaConEsqueletosDeTarjeta: mientras cargan, seis moldes con la medida
// de la tarjeta y en la misma rejilla, de modo que la primera tarjeta cae justo donde
// estaba el primer molde.
func TestLaPortadaCargaConEsqueletosDeTarjeta(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	p.AlCrear, p.AlElegir, p.AlEscanear = func() {}, func() {}, func() {}
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.MostrarPortada(nil, "es")
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))

	m := p.Portada.marco
	if m.cuerpo != nil {
		t.Fatalf("cargando, el cuerpo es %T; la rueda ya no va", m.cuerpo)
	}
	if len(m.esqueletos) != esqueletosRecientes {
		t.Fatalf("cargando hay %d esqueletos; la web pinta %d", len(m.esqueletos), esqueletosRecientes)
	}
	primero, segundo := m.esqueletos[0], m.esqueletos[1]
	if h := primero.Size().Height; h != editor.MedidaTesela() {
		t.Errorf("el molde mide %g de alto; la tarjeta, %g", h, editor.MedidaTesela())
	}
	molde := absoluta(primero)
	otro := absoluta(segundo)

	p.Portada.Lista = recientesDePrueba()
	p.Portada.Recomponer()
	w.Resize(fyne.NewSize(1280, 801))
	w.Resize(fyne.NewSize(1280, 800))
	m = p.Portada.marco
	if len(m.esqueletos) != 0 {
		t.Errorf("con la lista quedan %d esqueletos", len(m.esqueletos))
	}
	if a := absoluta(m.tarjetas[0]); a != molde || m.tarjetas[0].Size() != primero.Size() {
		t.Errorf("la primera tarjeta está en %v (%v) y su molde estaba en %v (%v)", a, m.tarjetas[0].Size(), molde, primero.Size())
	}
	if a := absoluta(m.tarjetas[1]); a != otro {
		t.Errorf("la segunda tarjeta está en %v y su molde estaba en %v", a, otro)
	}
}
