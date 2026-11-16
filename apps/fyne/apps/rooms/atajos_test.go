// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// hojaAbierta es el diálogo «Atajos de teclado» que haya en las capas, o nil.
func hojaAbierta(c fyne.Canvas, titulo string) *ui.Dialogo {
	for _, o := range c.Overlays().List() {
		if d := dialogoTitulado(o, titulo); d != nil {
			return d
		}
	}
	return nil
}

func dialogoTitulado(o fyne.CanvasObject, titulo string) *ui.Dialogo {
	if d, ok := o.(*ui.Dialogo); ok && d.Titulo == titulo {
		return d
	}
	var hijos []fyne.CanvasObject
	switch v := o.(type) {
	case *fyne.Container:
		hijos = v.Objects
	case fyne.Widget:
		hijos = test.WidgetRenderer(v).Objects()
	}
	for _, h := range hijos {
		if d := dialogoTitulado(h, titulo); d != nil {
			return d
		}
	}
	return nil
}

// pulsar teclea como el driver de escritorio: la runa es del control enfocado y solo
// sin foco llega a la ranura del lienzo (`test.TypeOnCanvas` se salta ese reparto).
func pulsar(c fyne.Canvas, s string) {
	for _, r := range s {
		if f := c.Focused(); f != nil {
			f.TypedRune(r)
		} else if g := c.OnTypedRune(); g != nil {
			g(r)
		}
	}
}

func TestElAtajoInterrogacionAbreLaHojaYEscapeLaCierra(t *testing.T) {
	sa, _, w := salaEn(t, servicioConDos(), 1280)
	sa.Atajos(w.Canvas())
	sa.Arrancar(context.Background())
	defer sa.Parar()
	c := w.Canvas()
	titulo := sa.P.T.T("shortcuts.title", nil)

	// Escribiendo en la caja «?» es texto, no un mando.
	caja := sa.P.Canal.Compositor.area
	c.Focus(caja)
	pulsar(c, "¿y?")
	if hojaAbierta(c, titulo) != nil {
		t.Fatal("«?» escrito en la caja ha abierto la hoja de atajos")
	}
	if !strings.Contains(sa.P.Canal.Compositor.Texto(), "?") {
		t.Fatalf("el «?» no ha llegado a la caja: %q", sa.P.Canal.Compositor.Texto())
	}

	c.Unfocus()
	pulsar(c, "?")
	d := hojaAbierta(c, titulo)
	if d == nil {
		t.Fatal("«?» sin foco no abre la hoja de atajos")
	}
	// Con la hoja puesta, otro «?» es de la capa: no se apila una segunda.
	if sa.Atajo(c, '?') {
		t.Error("con un diálogo abierto «?» no debe ser de la sala")
	}
	if f := c.Focused(); f != nil {
		f.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	} else {
		d.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	}
	if hojaAbierta(c, titulo) != nil {
		t.Fatal("Escape no cierra la hoja de atajos")
	}
	if sa.Atajo(c, 'x') {
		t.Error("una letra que no es atajo no debe ser de la sala")
	}
}

func TestLaHojaDeAtajosTraeLasDiezFilasDeLaWeb(t *testing.T) {
	sa, _, _ := salaEn(t, servicioConDos(), 1280)
	tr := sa.P.T
	grupos := sa.gruposDeAtajos()
	if len(grupos) != 2 || grupos[0].Titulo != tr.T("shortcuts.group.move", nil) ||
		grupos[1].Titulo != tr.T("shortcuts.group.write", nil) {
		t.Fatalf("grupos: %+v", grupos)
	}
	quiero := []ui.AtajoDeTeclado{
		{Teclas: []string{"Ctrl", "G"}, Descripcion: tr.T("shortcuts.switcher", nil)},
		{Teclas: []string{"Alt", "↑/↓"}, Descripcion: tr.T("shortcuts.nextChannel", nil)},
		{Teclas: []string{"Alt", tr.T("key.shift", nil), "↑/↓"}, Descripcion: tr.T("shortcuts.nextUnread", nil)},
		{Teclas: []string{"Esc"}, Descripcion: tr.T("shortcuts.close", nil)},
		{Teclas: []string{"?"}, Descripcion: tr.T("shortcuts.help", nil)},
		{Teclas: []string{tr.T("key.enter", nil)}, Descripcion: tr.T("composer.hintSend", nil)},
		{Teclas: []string{"↑"}, Descripcion: tr.T("shortcuts.editLast", nil)},
		{Teclas: []string{"@"}, Descripcion: tr.T("composer.hintMention", nil)},
		{Teclas: []string{":"}, Descripcion: tr.T("composer.hintEmoji", nil)},
		{Teclas: []string{"/"}, Descripcion: tr.T("composer.hintCommands", nil)},
	}
	var hay []ui.AtajoDeTeclado
	for _, g := range grupos {
		hay = append(hay, g.Atajos...)
	}
	if !slices.EqualFunc(hay, quiero, func(a, b ui.AtajoDeTeclado) bool {
		return a.Descripcion == b.Descripcion && slices.Equal(a.Teclas, b.Teclas)
	}) {
		t.Fatalf("filas:\n%+v\nquiero:\n%+v", hay, quiero)
	}
	for _, a := range hay {
		if strings.Contains(a.Descripcion, ".") && !strings.Contains(a.Descripcion, " ") {
			t.Errorf("descripción sin traducir: %q", a.Descripcion)
		}
	}
}
