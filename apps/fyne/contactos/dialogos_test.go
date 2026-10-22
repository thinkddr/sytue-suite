// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// campoDe busca el primer campo de texto de un árbol.
func campoDe(o fyne.CanvasObject) *ui.Entrada {
	switch x := o.(type) {
	case *ui.Entrada:
		return x
	case *fyne.Container:
		for _, h := range x.Objects {
			if e := campoDe(h); e != nil {
				return e
			}
		}
	}
	return nil
}

// botonesDe son los botones del pie, en orden.
func botonesDe(o fyne.CanvasObject) []*ui.Boton {
	var fuera []*ui.Boton
	switch x := o.(type) {
	case *ui.Boton:
		fuera = append(fuera, x)
	case *fyne.Container:
		for _, h := range x.Objects {
			fuera = append(fuera, botonesDe(h)...)
		}
	}
	return fuera
}

// TestNuevaEtiquetaPideNombreYLaCrea: sin nombre no sale nada y se dice por qué; con
// nombre, «Crear» la da de alta en la libreta activa, cierra y lo dice.
func TestNuevaEtiquetaPideNombreYLaCrea(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	a := appDePrueba(t, s)
	a.Cargar()
	d := a.dialogoEtiqueta(nil)
	if d.Titulo != "Nueva etiqueta" {
		t.Errorf("el título es %q", d.Titulo)
	}
	pie := botonesDe(d.Pie)
	if len(pie) != 2 || pie[0].Texto != "Cancelar" || pie[1].Texto != "Crear" {
		t.Fatalf("el pie es %v", pie)
	}
	pie[1].AlPulsar()
	if len(s.escrituras) != 0 || !contieneTexto(textosDe(d.Cuerpo), "La etiqueta necesita un nombre.") {
		t.Fatalf("sin nombre: escrituras %v, textos %v", s.escrituras, textosDe(d.Cuerpo))
	}
	campoDe(d.Cuerpo).PonerTexto("  Familia ")
	pie[1].AlPulsar()
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "POST /api/v1/address-books/l1/groups" ||
		s.escrituras[0].cuerpo != `{"name":"Familia"}` {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	if abierto(a) {
		t.Error("creada, el diálogo sigue abierto")
	}
	if av := a.avisos.Avisos(); len(av) != 1 || av[0].Titulo != "Etiqueta creada" {
		t.Errorf("avisos %v", av)
	}
}

// TestRenombrarEtiquetaConservaSusMiembros: el PUT sustituye la pertenencia, así que
// cambiar el nombre reenvía los miembros; mandar [] la dejaría vacía.
func TestRenombrarEtiquetaConservaSusMiembros(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("bruno", "Bruno")}}
	a := appDePrueba(t, s)
	a.Cargar()
	a.Pantalla.AlElegirEtiqueta("g1")
	d := a.dialogoEtiqueta(a.etiquetaActiva())
	if campoDe(d.Cuerpo).Texto() != "Amigos" || botonesDe(d.Pie)[1].Texto != "Guardar" {
		t.Fatalf("el diálogo de renombrar no parte del nombre: %q", campoDe(d.Cuerpo).Texto())
	}
	campoDe(d.Cuerpo).PonerTexto("Amistades")
	botonesDe(d.Pie)[1].AlPulsar()
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "PUT /api/v1/groups/g1" ||
		s.escrituras[0].cuerpo != `{"members":["u-bruno"],"name":"Amistades"}` {
		t.Fatalf("escrituras %v", s.escrituras)
	}
}

// TestBorrarEtiquetaPreguntaYQuitaElFiltro: la confirmación va en rojo y con el
// nombre; al borrarla, la lista deja de filtrar por ella.
func TestBorrarEtiquetaPreguntaYQuitaElFiltro(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("bruno", "Bruno")}}
	a := appDePrueba(t, s)
	a.Cargar()
	a.Pantalla.AlElegirEtiqueta("g1")
	a.Pantalla.AlBorrarEtiqueta()
	d, ok := capaDialogo(a)
	if !ok {
		t.Fatal("no se pidió confirmación")
	}
	if len(s.escrituras) != 0 {
		t.Fatalf("se borró antes de confirmar: %v", s.escrituras)
	}
	pie := botonesDe(d.Pie)
	if len(pie) != 2 || pie[1].Texto != "Eliminar etiqueta" || pie[1].Variante != ui.Peligro {
		t.Fatalf("el pie de la confirmación es %v", pie)
	}
	pie[1].AlPulsar()
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "DELETE /api/v1/groups/g1" {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	if a.Pantalla.EtiquetaActiva != "" {
		t.Error("borrada la etiqueta, la lista sigue filtrando por ella")
	}
}

// TestLaLibretaPredeterminadaNoSeBorra: el servicio la protege, así que su menú solo
// ofrece cambiar el nombre y ni siquiera se pregunta.
func TestLaLibretaPredeterminadaNoSeBorra(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	a := appDePrueba(t, s)
	a.Cargar()
	if !a.Pantalla.Activa.IsDefault {
		t.Fatal("la libreta de la escena no es la predeterminada")
	}
	a.Pantalla.AlBorrarLibreta()
	if abierto(a) || len(s.escrituras) != 0 {
		t.Error("se ofreció borrar la libreta predeterminada")
	}
}

// capaDialogo es el diálogo de encima del lienzo, si lo hay.
func capaDialogo(a *App) (*ui.Dialogo, bool) {
	top := a.lienzo().Overlays().Top()
	if top == nil {
		return nil, false
	}
	var buscar func(o fyne.CanvasObject) *ui.Dialogo
	buscar = func(o fyne.CanvasObject) *ui.Dialogo {
		if d, es := o.(*ui.Dialogo); es {
			return d
		}
		if w, es := o.(fyne.Widget); es {
			for _, h := range test.WidgetRenderer(w).Objects() {
				if d := buscar(h); d != nil {
					return d
				}
			}
		}
		return nil
	}
	d := buscar(top)
	return d, d != nil
}

// TestNuevaEtiquetaMideLoDeProduccion: LabelModal de ContactsApp.tsx usa Modal sm
// (400 px), FieldGroup e Input md. Con raíz de 16 y el diálogo del 2026-10-15: cabecera
// 61 (`pt-5 pb-2` + 32 + filete), cuerpo 84 (py-3 + rótulo18 + gap6 + input36), pie 65
// (filete + `pt-2 pb-5` + 36) y bordes exteriores 2: 212. Antes, 244.
func TestNuevaEtiquetaMideLoDeProduccion(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}})
	a.Cargar()
	d := a.dialogoEtiqueta(nil)
	if got := ui.MedidaDialogo(d, fyne.Size{}); got != fyne.NewSize(400, 212) {
		t.Errorf("«Nueva etiqueta» mide %v y en la web 400 × 212", got)
	}
}
