// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// todosLos recorre lo que pinta o (contenedores y widgets) y da lo que es de tipo T.
func todosLos[T any](o fyne.CanvasObject) []T {
	var fuera []T
	if x, ok := o.(T); ok {
		fuera = append(fuera, x)
	}
	switch v := o.(type) {
	case *fyne.Container:
		for _, h := range v.Objects {
			fuera = append(fuera, todosLos[T](h)...)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(v).Objects() {
			fuera = append(fuera, todosLos[T](h)...)
		}
	}
	return fuera
}

// Sytue Office (K-06): una libreta de una cuenta que no deja escribir no se edita aquí
// —lo que se cambiara no subiría—: ni crear, ni editar, ni etiquetar, ni combinar, ni
// importar, ni mandar a la papelera; destacar sí, que es de este equipo. Y lo dice
// sobre la lista. La misma libreta con permiso de escritura, como siempre.
func TestLibretaDeSoloLecturaNoSeEditaAqui(t *testing.T) {
	p := &Pantalla{T: traductor(t)}
	p.Libretas = []api.AddressBook{{ID: "b1", DisplayName: "iCloud"}}
	p.Activa = &p.Libretas[0]
	ana := persona("ana", "Ana")
	p.Contactos, p.Total, p.Seleccionado = []api.Contact{ana, persona("bea", "Bea")}, 2, &ana
	p.Etiquetas = []api.ContactGroup{{ID: "g1", Name: "Familia"}}
	p.Lote = []api.Contact{ana}
	nada := func() {}
	p.AlCrearContacto, p.AlLote = nada, func(AccionLote, []api.Contact, *api.ContactGroup) {}
	p.AlEditar, p.AlCombinar, p.AlEtiquetar = func(api.Contact) {}, func(api.Contact) {}, func(api.ContactGroup, api.Contact) {}
	const aviso = "Esta libreta es de solo lectura en su cuenta."
	cuenta := GrupoCuenta{Rotulo: "iCloud", Libretas: []string{"b1"}, SoloLectura: map[string]bool{"b1": true}, AvisoSoloLectura: aviso}

	editar, papelera := p.t("common.edit", nil), p.t("batch.toTrash", nil)
	tiene := func(bs []*ui.Boton, texto string) bool {
		for _, b := range bs {
			if b.Texto == texto {
				return true
			}
		}
		return false
	}
	franja := func() *ui.AyudaEnLinea {
		for _, f := range todosLos[*ui.AyudaEnLinea](p.lista()) {
			return f
		}
		return nil
	}

	// Con permiso de escritura: todo como siempre.
	cuenta.SoloLectura["b1"] = false
	p.Cuentas = []GrupoCuenta{cuenta}
	if !p.escribe() || !tiene(todosLos[*ui.Boton](p.accionesFicha(ana)), editar) || franja() != nil {
		t.Fatal("una libreta que se puede escribir se trata como de solo lectura")
	}
	if lote := p.barraLote(); lote == nil || !tiene(lote.Acciones, papelera) {
		t.Fatal("la barra de lote de siempre")
	}

	cuenta.SoloLectura["b1"] = true
	p.Cuentas = []GrupoCuenta{cuenta}
	if p.escribe() {
		t.Fatal("se puede escribir en una libreta de solo lectura")
	}
	if tiene(todosLos[*ui.Boton](p.accionesFicha(ana)), editar) {
		t.Error("la ficha ofrece «Editar»")
	}
	for _, b := range todosLos[*ui.Boton](p.menu()) {
		if b.Texto == p.t("nav.newContact", nil) && !b.Inerte {
			t.Error("«Nuevo contacto» sigue vivo")
		}
	}
	if f := franja(); f == nil || f.Cuerpo != aviso {
		t.Errorf("sin aviso sobre la lista: %+v", f)
	}
	lote := p.barraLote()
	if lote == nil || tiene(lote.Acciones, papelera) || !tiene(lote.Acciones, p.t("batch.star", nil)) {
		t.Error("la barra de lote: destacar sí, papelera no")
	}
}
