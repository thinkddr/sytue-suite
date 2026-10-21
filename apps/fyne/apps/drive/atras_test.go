// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestElAtrasSigueSeleccionBusquedaYCarpetas(t *testing.T) {
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(traductor(t), nil, nil, w)
	p.aparte = func(f func()) { f() }
	w.SetContent(p.Objeto())
	p.Arrancar(false)
	atras := func() { w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack}) }
	e := p.explor
	e.Ruta = []Elemento{{ID: "a", Nombre: "A"}, {ID: "b", Nombre: "B"}}
	e.Elementos = []Elemento{{ID: "f", Nombre: "F"}}
	e.Seleccion["f"] = true
	e.EnBusqueda, e.Consulta = true, "acta"
	atras()
	if e.Marcados() != 0 || !e.EnBusqueda || len(e.Ruta) != 2 {
		t.Fatal("atrás debe limpiar primero la selección")
	}
	atras()
	if e.EnBusqueda || e.Consulta != "" || len(e.Ruta) != 2 {
		t.Fatal("atrás debe salir de la búsqueda antes de subir carpeta")
	}
	atras()
	if len(e.Ruta) != 1 || e.Ruta[0].ID != "a" {
		t.Fatal("atrás no usa la miga de la carpeta superior")
	}
	atras()
	if len(e.Ruta) != 0 || p.atras() {
		t.Fatal("en la raíz atrás debe quedar para el sistema")
	}
	p.irA(Favoritos)
	atras()
	if p.seccion != MiUnidad {
		t.Fatal("atrás desde una sección no vuelve a Mi unidad")
	}
}

func TestElAtrasDeCompartidosYEquiposUsaSuCarpetaSuperior(t *testing.T) {
	p := NuevaPantalla(traductor(t), nil, nil, nil)
	p.seccion = CompartidoConmigo
	c := p.sec.compartido
	c.Raiz, c.Subcamino = &api.Compartido{File: api.File{ID: "s1"}}, "Actas/2026"
	c.AlIr = func(ruta string) { c.Subcamino = ruta }
	c.AlVolver = func() { c.Raiz = nil }
	p.atras()
	if c.Subcamino != "Actas" || c.Raiz == nil {
		t.Fatal("debe subir dentro del compartido")
	}
	p.atras()
	p.atras()
	if c.Raiz != nil || p.seccion != CompartidoConmigo {
		t.Fatal("la raíz debe volver a Compartido conmigo")
	}
	p.seccion = UnidadesDeEquipo
	q := p.sec.equipos
	q.Abierta = &api.UnidadEquipo{ID: "u1"}
	q.Pila = []Elemento{{ID: "a"}, {ID: "b"}}
	q.AlIr = func(nivel int) { q.Pila = q.Pila[:nivel+1] }
	q.AlVolver = func() { q.Abierta = nil }
	p.atras()
	if len(q.Pila) != 1 || q.Abierta == nil {
		t.Fatal("debe subir dentro de la unidad de equipo")
	}
	p.atras()
	p.atras()
	if q.Abierta != nil || p.seccion != UnidadesDeEquipo {
		t.Fatal("la raíz debe volver a Unidades de equipo")
	}
}
