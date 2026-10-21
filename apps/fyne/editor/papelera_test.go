// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"net/http"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

const papeleraDeDos = `{"documents":[` +
	`{"id":"a","kind":"document","title":"Acta","deleted_at":"2026-09-28T10:00:00Z"},` +
	`{"id":"b","kind":"document","title":"Borrador","deleted_at":"2026-09-27T10:00:00Z"}]}`

// TestLaPortadaAbreLaPapelera: «Papelera» enseña la papelera del producto —la misma
// petición que `DocTrash.tsx`, con su `?kind=`— y volver trae la lista al día.
func TestLaPortadaAbreLaPapelera(t *testing.T) {
	s := &servicio{lista: dosDocs, papelera: papeleraDeDos}
	p := portadaDePrueba(t, s)
	p.Cargar()
	p.AbrirPapelera()
	x := p.Papelera()
	if x == nil || s.ultimo() != "GET /api/v1/trash?kind=document" {
		t.Fatalf("abrir la papelera pide %q", s.ultimo())
	}
	if x.Estado != Listo || len(x.Docs) != 2 || x.Docs[0].DeletedAt == nil {
		t.Fatalf("papelera: estado %v, %d documentos", x.Estado, len(x.Docs))
	}
	p.Objeto().Resize(fyne.NewSize(900, 700))
	pinta(t, 900, 700, p.Objeto())
	x.AlVolver()
	if p.Papelera() != nil || s.ultimo() != "GET /api/v1/documents?kind=document&limit=100" {
		t.Fatalf("volver no recarga la portada: %q", s.ultimo())
	}
}

// TestRestaurarPurgarYVaciar: cada acción pide lo suyo y recarga; si el servicio dice
// que no, la lista no cambia.
func TestRestaurarPurgarYVaciar(t *testing.T) {
	s := &servicio{papelera: papeleraDeDos}
	p := portadaDePrueba(t, s)
	p.AbrirPapelera()
	x := p.Papelera()

	x.Restaurar(x.Docs[0])
	if !strings.Contains(strings.Join(s.pedido, "\n"), "POST /api/v1/trash/a/restore") ||
		s.ultimo() != "GET /api/v1/trash?kind=document" {
		t.Fatalf("restaurar: %q", s.pedido)
	}
	s.mu.Lock()
	s.restaurar = http.StatusConflict
	s.mu.Unlock()
	antes := len(s.pedido)
	x.Restaurar(x.Docs[0])
	if len(s.pedido) != antes+1 || x.Ocupado {
		t.Fatalf("con el nombre ocupado no se recarga y se suelta el botón: %q", s.pedido[antes:])
	}

	x.Purgar(x.Docs[1])
	if !strings.Contains(strings.Join(s.pedido, "\n"), "DELETE /api/v1/trash/b?") {
		t.Fatalf("purgar: %q", s.pedido)
	}
	x.Vaciar()
	var paginas []string
	for _, q := range s.pedido {
		if strings.HasPrefix(q, "DELETE /api/v1/trash?") {
			paginas = append(paginas, q)
		}
	}
	if len(paginas) != 2 || !strings.Contains(paginas[1], "after=") || !strings.Contains(paginas[0], "kind=document") {
		t.Fatalf("vaciar no siguió por páginas: %q", paginas)
	}
}

// TestLaPapeleraPintaCadaEstado: ningún estado entra en pánico al pintar.
func TestLaPapeleraPintaCadaEstado(t *testing.T) {
	for _, e := range []Estado{Cargando, Vacio, Error, SinConexion, SinPermiso, SinSesion, Listo} {
		x := NuevaPapeleraDocs(Hoja, nil, "es", nil)
		x.Estado = e
		x.Repintar()
		x.Objeto().Resize(fyne.NewSize(360, 640))
		pinta(t, 360, 640, x.Objeto())
	}
}

// TestLaPapeleraVaEnLaCabecera: «Papelera» va arriba a la derecha de la cabecera, y a
// 360 cabe sin pisar el título. En la fila de «Recientes» no cabía: «Destacados»
// acababa encima de la insignia (VISTO EN ROJO con TestA360RecientesNoQuedaTapado).
func TestLaPapeleraVaEnLaCabecera(t *testing.T) {
	s := &servicio{lista: dosDocs}
	p := portadaDePrueba(t, s)
	p.Cargar()
	for _, ancho := range []float32{360, 1280} {
		ventana := test.NewWindow(p.Objeto())
		ventana.SetPadded(false)
		ventana.Resize(fyne.NewSize(ancho, 740))
		w := p.vista
		pap := w.papelera
		caja := min(ancho, tema.AnchoContenido)
		der := (ancho-caja)/2 + caja - MargenPortada
		if fin := pap.Position().X + pap.Size().Width; fin != der || pap.Position().Y != ArribaPortada {
			t.Errorf("a %g la papelera acaba en %g (quiero %g) y empieza en y=%g", ancho, fin, der, pap.Position().Y)
		}
		titulo := w.r.titulo
		if fin := titulo.Position().X + ui.Medir(titulo.Text, tema.Texto2XL, ui.Negrita).Width; fin > pap.Position().X {
			t.Errorf("a %g el título acaba en %g y pisa la papelera (%g)", ancho, fin, pap.Position().X)
		}
		ventana.Close()
	}
}

// TestUnaPapeleraCerradaNoRecarga: lo que responde tarde, tras volver a la portada, no
// pide otra vez la papelera.
func TestUnaPapeleraCerradaNoRecarga(t *testing.T) {
	s := &servicio{papelera: papeleraDeDos}
	p := portadaDePrueba(t, s)
	p.AbrirPapelera()
	x := p.Papelera()
	p.CerrarPapelera()
	antes := len(s.pedido)
	x.Cargar()
	if len(s.pedido) != antes {
		t.Fatalf("la papelera cerrada volvió a pedir: %q", s.pedido[antes:])
	}
}
