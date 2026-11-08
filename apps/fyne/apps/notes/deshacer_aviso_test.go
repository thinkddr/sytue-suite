// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// ultimoAviso es el aviso más reciente de la pila.
func ultimoAviso(t *testing.T, p *Pantalla) *ui.Aviso {
	t.Helper()
	as := p.avisos.Avisos()
	if len(as) == 0 {
		t.Fatal("no ha salido ningún aviso")
	}
	return as[len(as)-1]
}

// esperarVeces espera a que el servicio falso vea `veces` peticiones `clave`: «Deshacer»
// del aviso habla con el servicio en su propia rutina.
func esperarVeces(t *testing.T, falso *servicioFalso, clave string, veces int) {
	t.Helper()
	for range 300 {
		fyne.DoAndWait(func() {})
		n := 0
		for _, r := range falso.rutas() {
			if r == clave {
				n++
			}
		}
		if n >= veces {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no han llegado %d × %s; llegaron %v", veces, clave, falso.rutas())
}

// TestEliminarAvisaYDeshacerDelAvisoLaRestaura: eliminar deja «Nota enviada a la
// papelera» con «Deshacer», que la saca de la papelera aunque después se haya hecho
// otra cosa —deshace ESE paso, no el último—. VISTO EN ROJO quitando el aviso de
// `borrar` y deshaciendo con `Deshacer` en vez de `DeshacerEsta`.
func TestEliminarAvisaYDeshacerDelAvisoLaRestaura(t *testing.T) {
	p, falso := pantallaConServicio(t, nil, api.Note{ID: "a", NotebookID: "c1"}, api.Note{ID: "b", NotebookID: "c1"})
	if err := p.borrar("a", true); err != nil {
		t.Fatal(err)
	}
	a := ultimoAviso(t, p)
	if a.Titulo != p.tr.T("note.trashed", nil) || a.RotuloAccion != p.tr.T("history.undo", nil) || a.AlPulsarAccion == nil {
		t.Fatalf("el aviso de eliminar es %q con acción %q", a.Titulo, a.RotuloAccion)
	}
	// Otra cosa entre medias: el aviso no debe deshacer esta.
	p.hist.Registrar(Accion{Etiqueta: "otra", Deshacer: func() error { return nil }}, "")

	a.AlPulsarAccion()
	esperarVeces(t, falso, "POST /notes/a/restore", 1)
	for range 100 {
		fyne.DoAndWait(func() {})
		if slices.ContainsFunc(p.notas, func(n Nota) bool { return n.ID == "a" }) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !slices.ContainsFunc(p.notas, func(n Nota) bool { return n.ID == "a" }) {
		t.Fatalf("después de deshacer desde el aviso: notas %s", ids(p.notas))
	}
	if got := p.hist.SiguienteDeshacer(); got != "otra" {
		t.Fatalf("el aviso ha deshecho lo que no era: lo siguiente es %q", got)
	}
}

// TestMoverDeCuadernoAvisaConDeshacer: el aviso «Movida a …» lleva «Deshacer», que la
// devuelve al cuaderno de origen.
func TestMoverDeCuadernoAvisaConDeshacer(t *testing.T) {
	p, falso := pantallaConServicio(t, nil, api.Note{ID: "a", NotebookID: "c1"})
	if err := p.moverACuaderno("a", p.cuadernos[1], true); err != nil {
		t.Fatal(err)
	}
	a := ultimoAviso(t, p)
	if a.RotuloAccion != p.tr.T("history.undo", nil) || a.AlPulsarAccion == nil {
		t.Fatalf("el aviso de mover %q no ofrece deshacer", a.Titulo)
	}
	a.AlPulsarAccion()
	esperarVeces(t, falso, "POST /notes/a/move", 2)
	if destino := falso.ultima(t, "POST /notes/a/move")["notebook_id"]; destino != "c1" {
		t.Fatalf("deshacer la manda a %v y no al cuaderno de origen", destino)
	}
}

// TestDeshacerDelAvisoTrasCambiarDeCuaderno: se elimina en «Mudanza», se abre «Trabajo»
// y se pulsa el «Deshacer» del aviso, que sigue a la vista. La nota sale de la papelera
// pero NO se pinta en «Trabajo», que no es su cuaderno. VISTO EN ROJO quitando la
// comprobación del cuaderno en `añadir`.
func TestDeshacerDelAvisoTrasCambiarDeCuaderno(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"GET /notebooks/c2/notes/page": `{"notes":[{"id":"z","notebook_id":"c2","kind":"text"}]}`,
	}, api.Note{ID: "a", NotebookID: "c1"})
	if err := p.borrar("a", true); err != nil {
		t.Fatal(err)
	}
	a := ultimoAviso(t, p)
	fyne.DoAndWait(func() { p.abrirCuaderno(p.cuadernos[1]) })
	for range 300 {
		var listo bool
		fyne.DoAndWait(func() { _, listo = p.nota("z") })
		if listo {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.AlPulsarAccion()
	esperarVeces(t, falso, "POST /notes/a/restore", 1)
	for range 20 {
		fyne.DoAndWait(func() {})
		time.Sleep(10 * time.Millisecond)
	}
	var got string
	fyne.DoAndWait(func() { got = ids(p.notas) })
	if got != "z" {
		t.Fatalf("en «Trabajo» después de deshacer: notas %s", got)
	}
}
