// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Ctrl/Cmd +, − y 0 sobre el zoom (`atajosZoom.ts` de la web), y el «100 %» de la barra
// de estado, que vuelve al 100 con un clic (`StatusBar.tsx`).
//
// VISTO EN ROJO (2026-11-05): sin `AtajosDelEditor` en la ventana del marco, Ctrl+= no
// movía el zoom; con el porcentaje como texto, no había botón que pulsar.
func TestElZoomConElTecladoYElPorcentajeQueVuelveAlCien(t *testing.T) {
	ctrl := fyne.KeyModifierShortcutDefault
	atajo := func(k fyne.KeyName, m fyne.KeyModifier) *desktop.CustomShortcut {
		return &desktop.CustomShortcut{KeyName: k, Modifier: m}
	}
	ctx := &Contexto{Tipo: Hoja, Estado: Listo, Zoom: 100}
	m := NuevoMarco(ctx, Genericas(), nil)
	w := test.NewTempWindow(t, m)
	w.Resize(fyne.NewSize(1280, 800))
	// El lienzo de prueba envuelve al de software, que es el que reparte los atajos
	// registrados con AddShortcut, como la ventana de verdad.
	var lienzo interface{ TypedShortcut(fyne.Shortcut) }
	if v := reflect.ValueOf(w.Canvas()).Elem(); v.Kind() == reflect.Struct && v.NumField() > 0 {
		lienzo, _ = v.Field(0).Interface().(interface{ TypedShortcut(fyne.Shortcut) })
	}
	if lienzo == nil {
		t.Fatal("el lienzo de prueba no recibe atajos")
	}
	for _, paso := range []struct {
		a      *desktop.CustomShortcut
		quiero int
	}{
		{atajo(fyne.KeyEqual, ctrl), 110},
		{atajo(fyne.KeyEqual, ctrl|fyne.KeyModifierShift), 120}, // el + de muchos teclados
		{atajo(fyne.KeyPlus, ctrl), 130},
		{atajo(fyne.KeyMinus, ctrl), 120},
		{atajo(fyne.Key0, ctrl), 100},
	} {
		lienzo.TypedShortcut(paso.a)
		if ctx.Zoom != paso.quiero {
			t.Fatalf("%s: zoom %d; quiero %d", paso.a.ShortcutName(), ctx.Zoom, paso.quiero)
		}
	}
	// Sin Ctrl, o con Alt, no es del zoom: el lienzo con el foco lo deja pasar.
	for _, a := range []*desktop.CustomShortcut{atajo(fyne.KeyEqual, fyne.KeyModifierAlt), atajo(fyne.Key0, ctrl|fyne.KeyModifierAlt)} {
		if ctx.AtajoDelEditor(a) {
			t.Errorf("%s no es un atajo del editor", a.ShortcutName())
		}
	}
	ctx.PonerZoom(ZoomMax)
	ctx.AtajoDelEditor(atajo(fyne.KeyPlus, ctrl))
	if ctx.Zoom != ZoomMax {
		t.Errorf("el atajo se sale del techo: %d", ctx.Zoom)
	}

	// El porcentaje de la barra: se lee el zoom, se anuncia como botón y vuelve al 100.
	ctx.PonerZoom(150)
	p := m.estado.porciento
	if p.Texto != "150 %" || p.Nombre != "status.zoom.reset" || !p.Boton {
		t.Fatalf("el porcentaje: %q, nombre %q, botón %v", p.Texto, p.Nombre, p.Boton)
	}
	test.Tap(p)
	if ctx.Zoom != 100 || p.Texto != "100 %" {
		t.Errorf("pulsar el porcentaje deja el zoom en %d (%q)", ctx.Zoom, p.Texto)
	}
	// Del 90 al 100 el rótulo crece: el marco recoloca la barra y no lo corta con elipsis.
	ctx.PonerZoom(90)
	ctx.PonerZoom(100)
	if p.Size().Width < ui.MedidaEnlace(p).Width {
		t.Errorf("el porcentaje se queda con el ancho de antes: %v", p.Size())
	}
}
