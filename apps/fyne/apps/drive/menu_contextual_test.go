// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestElClicDerechoDeLaTarjetaAbreSuMenu: como `onContextMenu` en las teselas de
// `FileGrid`, el clic derecho (o mantener pulsado) abre el menú del «⋯» de esa tarjeta
// sin abrirla, aunque la esquina aún no se vea. Sin menú, nada.
//
// VISTO EN ROJO (2026-11-08): sin `TappedSecondary`, la tarjeta no atiende el clic
// derecho.
func TestElClicDerechoDeLaTarjetaAbreSuMenu(t *testing.T) {
	tj := NuevaTarjetaDrive("playa.jpg", "jpg", false)
	var abierta, menus int
	var colgado fyne.CanvasObject
	tj.AlAbrir = func() { abierta++ }
	tj.AlMenu = func(d fyne.CanvasObject) { menus++; colgado = d }
	w := test.NewWindow(tj)
	defer w.Close()
	tam := MedidaTarjetaDrive(200)
	w.Resize(tam)
	tj.Resize(tam)

	var obj fyne.CanvasObject = tj
	sec, ok := obj.(fyne.SecondaryTappable)
	if !ok {
		t.Fatal("la tarjeta no responde al clic derecho")
	}
	test.TapSecondary(sec)
	if menus != 1 || abierta != 0 {
		t.Fatalf("el clic derecho abre %d menús y %d tarjetas; quiero 1 y 0", menus, abierta)
	}
	if o, _ := cajaMasTarjeta(tj.Size()); colgado == nil || colgado.Position() != o {
		t.Errorf("el menú no cuelga del «⋯»: %v", colgado)
	}

	sin := NuevaTarjetaDrive("viejo.jpg", "jpg", false)
	sin.AlAbrir = func() { abierta++ }
	w2 := test.NewWindow(sin)
	defer w2.Close()
	w2.Resize(tam)
	if sec, ok := any(sin).(fyne.SecondaryTappable); ok {
		test.TapSecondary(sec)
	}
	if abierta != 0 {
		t.Error("sin menú, el clic derecho abre la tarjeta")
	}
}

// TestElClicDerechoDeLaFilaPlanaAbreSuMenu: en «Recientes» y «Destacados» el clic
// derecho dispara el «⋯» de la fila —no la estrella, que va antes— con el elemento de
// esa fila. Una fila sin «⋯» (las de «Compartido conmigo») no hace nada.
//
// VISTO EN ROJO (2026-11-08): sin `TappedSecondary` ni `BotonFila.Menu`, el clic
// derecho no abre nada.
func TestElClicDerechoDeLaFilaPlanaAbreSuMenu(t *testing.T) {
	tr := traductor(t)
	vp := NuevaVistaPlana(tr, Recientes, map[string]bool{})
	var menu []string
	var estrellas int
	vp.AlMenu = func(el Elemento, _ fyne.CanvasObject) { menu = append(menu, el.ID) }
	vp.AlEstrella = func(Elemento, bool) { estrellas++ }
	f := vp.fila(Elemento{ID: "f2", Nombre: "informe.pdf", Ruta: "/informe.pdf"})
	w := test.NewWindow(f)
	defer w.Close()
	w.Resize(fyne.NewSize(800, f.MinSize().Height))

	var obj fyne.CanvasObject = f
	sec, ok := obj.(fyne.SecondaryTappable)
	if !ok {
		t.Fatal("la fila plana no responde al clic derecho")
	}
	test.TapSecondary(sec)
	if len(menu) != 1 || menu[0] != "f2" || estrellas != 0 {
		t.Fatalf("el clic derecho abre los menús %q y %d estrellas; quiero [f2] y 0", menu, estrellas)
	}

	sin := NuevaFilaPlana("plano.pdf")
	var pulsado int
	sin.Botones = []BotonFila{{Cuerpo: glifoDescargar, Tinta: "text-muted", AlPulsar: func(fyne.CanvasObject) { pulsado++ }}}
	w2 := test.NewWindow(sin)
	defer w2.Close()
	w2.Resize(fyne.NewSize(800, sin.MinSize().Height))
	if sec, ok := any(sin).(fyne.SecondaryTappable); ok {
		test.TapSecondary(sec)
	}
	if pulsado != 0 {
		t.Error("una fila sin «⋯» dispara otro botón con el clic derecho")
	}
}
