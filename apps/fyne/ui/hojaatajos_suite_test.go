// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// «?» EN TODAS LAS APPS (`HojaDeAtajos.tsx`): la barra de la suite engancha la tecla al
// lienzo y abre la hoja con los atajos de la app y el grupo de la suite.
//
// Visto en ROJO: sin `registrarAyuda` en el Layout de la barra, «?» no abre nada.

// ventanaConBarra monta la barra sobre un contenido vacío y la deja colocada.
func ventanaConBarra(t *testing.T, antes func(fyne.Canvas)) (*BarraSuite, fyne.Canvas) {
	t.Helper()
	b, parar := BarraSuiteDePrueba(traductorPrueba(t), iconos.Notes)
	t.Cleanup(parar)
	w := test.NewTempWindow(t, canvas.NewRectangle(color.NRGBA{}))
	if antes != nil {
		antes(w.Canvas())
	}
	w.SetContent(b.Montar(canvas.NewRectangle(color.NRGBA{})))
	w.Resize(fyne.NewSize(1280, 800))
	w.Canvas().Capture()
	return b, w.Canvas()
}

func hojasAbiertas(c fyne.Canvas) []*Dialogo {
	var hojas []*Dialogo
	var mirar func(fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		if d, ok := o.(*Dialogo); ok {
			hojas = append(hojas, d)
		}
		switch x := o.(type) {
		case *fyne.Container:
			for _, h := range x.Objects {
				mirar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				mirar(h)
			}
		}
	}
	for _, o := range c.Overlays().List() {
		mirar(o)
	}
	return hojas
}

func teclear(c fyne.Canvas, r rune) {
	fyne.DoAndWait(func() {
		if f := c.OnTypedRune(); f != nil {
			f(r)
		}
	})
}

func TestElInterroganteAbreLaHojaDeLaSuite(t *testing.T) {
	b, c := ventanaConBarra(t, nil)
	b.ConAtajosDeApp(func() []GrupoDeAtajos {
		return []GrupoDeAtajos{{Titulo: "Notas", Atajos: []AtajoDeTeclado{
			{Teclas: []string{"n"}, Descripcion: "Nueva nota"},
			{Teclas: []string{"?"}, Descripcion: "Ver esta lista"},
		}}}
	})
	teclear(c, 'x')
	if n := len(hojasAbiertas(c)); n != 0 {
		t.Fatalf("una tecla cualquiera abre %d hojas", n)
	}
	teclear(c, '?')
	hojas := hojasAbiertas(c)
	if len(hojas) != 1 {
		t.Fatalf("«?» abre %d hojas, no una", len(hojas))
	}
	if want := textoComun("shell.shortcuts.title"); hojas[0].Titulo != want {
		t.Errorf("la hoja se titula %q, no %q", hojas[0].Titulo, want)
	}
	textos := textosDeArbol(hojas[0])
	for _, s := range []string{"Nueva nota", textoComun("shell.shortcuts.search"), textoComun("shell.shortcuts.close")} {
		if !strings.Contains(textos, s) {
			t.Errorf("la hoja no enseña %q:\n%s", s, textos)
		}
	}
	if n := strings.Count(textos, textoComun("shell.shortcuts.help")); n != 1 {
		t.Errorf("«Ver esta lista» sale %d veces: la fila de la app se repite", n)
	}
	// Con la hoja abierta, otro «?» no abre otra encima: el teclado es de la capa.
	teclear(c, '?')
	if n := len(hojasAbiertas(c)); n != 1 {
		t.Errorf("un segundo «?» deja %d hojas", n)
	}
}

// La app va primero: si con su «?» ya abrió una hoja (Mail, Calendar…), la de la
// suite no se pone encima; y sus demás teclas le siguen llegando.
func TestLaAppVaPrimeroConElInterrogante(t *testing.T) {
	var propias []rune
	_, c := ventanaConBarra(t, func(c fyne.Canvas) {
		c.SetOnTypedRune(func(r rune) {
			propias = append(propias, r)
			if r == '?' {
				MostrarHojaAtajos(c, "Mía", "Cerrar", nil)
			}
		})
	})
	teclear(c, 'j')
	teclear(c, '?')
	if string(propias) != "j?" {
		t.Errorf("la app recibe %q, no «j?»", string(propias))
	}
	hojas := hojasAbiertas(c)
	if len(hojas) != 1 || hojas[0].Titulo != "Mía" {
		t.Errorf("con la hoja de la app abierta hay %d hojas", len(hojas))
	}
}

func TestComponerHojaDeSuiteQuitaElInterroganteDeLaApp(t *testing.T) {
	hoja := ComponerHojaDeSuite("", []GrupoDeAtajos{{Titulo: "Ayuda", Atajos: []AtajoDeTeclado{{Teclas: []string{"?"}}}}})
	if len(hoja) != 1 || hoja[0].Titulo != textoComun("shell.shortcuts.group") {
		t.Errorf("la hoja compuesta es %+v", hoja)
	}
}
